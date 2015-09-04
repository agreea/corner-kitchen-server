package main

import (
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha256"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

type KitchenUserServlet struct {
	db              *sql.DB
	random          *rand.Rand
	server_config   *Config
	session_manager *SessionManager
}

func NewKitchenUserServlet(server_config *Config, session_manager *SessionManager) *KitchenUserServlet {
	t := new(KitchenUserServlet)
	t.random = rand.New(rand.NewSource(time.Now().UnixNano()))

	t.session_manager = session_manager
	t.server_config = server_config

	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewKitchenUserServlet", "Failed to open database:", err)
	}
	t.db = db

	return t
}

// TODO: Implement
func (t *KitchenUserServlet) Add_stripe_token(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println("Validate", err)
		return nil
	}
	if !session_valid {
		return APIError("Session not valid", 401)
	}

	token := new(PaymentToken)
	token.User_id = session.User.Id
	token.Name = r.Form.Get("token_name")
	token.stripe_key = r.Form.Get("stripe_token")
	token.Token = uuid.New()
	err = SavePaymentToken(t.db, token)

	if err != nil {
		log.Println(err)
		return nil
	}

	token, err = GetPaymentToken(t.db, token.Token)
	if err != nil {
		log.Println(err)
		return nil
	}

	return APISuccess(token)
}

// Create a login session for a user.
// Session tokens are stored in a local cache, as well as back to the DB to
// support multi-server architecture. A cache miss will result in a DB read.
func (t *KitchenUserServlet) Login(r *http.Request) *ApiResult {
	// if you don't have the fb in your guest table, 
		// create a long-lived access token from the short lived one
	fbToken := r.Form.Get("fbToken")
	resp, err := t.get_fb_data_for_token(fbToken)
	if err != nil {
		return APIError("Invalid Facebook Login", 400)
	}
	if resp["error"] != nil {
		return APIError("Error connecting to Facebook", 500)
	}
	return APISuccess("hey there")
	fbId := resp["id"].(int64)
	fb_id_exists, err := t.fb_id_exists(resp["id"].(int64))
	if err != nil {
		return APIError("Could not login", 500)
	}
    if fb_id_exists {
    	guestData, err := t.process_login(fbId)
    	if err != nil {
			return APIError("Could not login", 500)
    	}
		return APISuccess(guestData)
	} else {
		guestData, err := t.create_guest(resp["email"].(string), resp["name"].(string), resp["id"].(int))
		if err != nil {
			return APIError("Failed to create user", 500)
		}
		return APISuccess(guestData)
	}
}

// Returns json data
// Todo: json encoding response body contents
func (t *KitchenUserServlet) get_fb_data_for_token(fb_token string) (resp *FacebookResp, err error) {
	resp, err = http.Get("https://graph.facebook.com/me?fields=id,name,email&access_token=" + fb_token)
	if err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()
		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		} else {
			err := json.Unmarshal(contents, &resp)
			if err != nil{
				return nil, err
			} else {
				return resp, nil
			}
		}
	}
}


func (t *KitchenUserServlet) Get(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 200)
	}
	return APISuccess(session.User)
}

func (t *KitchenUserServlet) Delete(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 200)
	}
	_, err = t.db.Exec("DELETE FROM User where Id = ?", session.User.Id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	return APISuccess("OK")
}

// Fetches a user's data and creates a session for them.
// Returns a pointer to the userdata and an error.
func (t *KitchenUserServlet) process_login(fbid int) (*GuestData, error) {
	guestData, err := GetGuestByFbId(t.db, int64(fbid))
	if err != nil {
		return nil, err
	}
	guest_session, err := t.session_manager.GetGuestSessionById(guestData.Id)
	if err != nil {
		return nil, err
	}
	if guest_session != "" {
		guestData.Session_token = guest_session
		return guestData, nil
	} else {
		guestData.Session_token, err = t.session_manager.CreateSessionForGuest(guestData.Id)
		if err != nil {
			return nil, err
		}
		return guestData, nil
	}
}
// Create a new user + session based off of the data returned from facebook and return a GuestData object
func (t *KitchenUserServlet) create_guest(email string, name string, fb_id int) (*GuestData, error) {
	prof_pic_url := "http://graph.facebook.com/" + strconv.Itoa(fb_id) + "/picture?width=400"
	_, err := t.db.Exec(`INSERT INTO Guest
		(Email, Name, FbId, profpic) VALUES (?, ?, ?, ?)`,
		email, name, fb_id, prof_pic_url)
	guestData, err := GetGuestByFbId(t.db, int64(fb_id))
	if err != nil {
		log.Println("Create guest", err)
	}
	guestData.Session_token, err = t.session_manager.CreateSessionForGuest(guestData.Id)
	if err != nil {
		log.Println("Create guest", err)
		return nil, err
	}
	return guestData, nil
}

// Check if a fbId already exists in the chakula DB.
// Returns an error if any database operation fails.
func (t *KitchenUserServlet) fb_id_exists(fb_id int) (bool, error) {
	rows, err := t.db.Query("SELECT Id FROM Guest WHERE FbId = ?", int64(fb_id))
	if err != nil {
		return true, err
	}
	defer rows.Close()
	num_rows := 0
	for rows.Next() {
		num_rows++
		var id int
		if err := rows.Scan(&id); err != nil {
			return true, err
		}
	}
	if err := rows.Err(); err != nil {
		return true, err
	}
	if num_rows > 0 {
		return true, nil
	} else {
		return false, nil
	}
}

// Create a random bytestring
func (t *KitchenUserServlet) generate_random_bytestring(length int) []byte {
	random_bytes := make([]byte, length)
	for i := range random_bytes {
		random_bytes[i] = byte(t.random.Int() & 0xff)
	}
	return random_bytes
}

// Create a random alphanumeric string
func (t *KitchenUserServlet) generate_random_alphanumeric(length int) []byte {
	random_bytes := make([]byte, length)

	for i := range random_bytes {
		random_bytes[i] = alphanumerics[t.random.Int()%len(alphanumerics)]
	}
	return random_bytes
}

// Generate a PBKDF password hash. Use 4096 iterations and a 64 byte key.
func (t *KitchenUserServlet) generate_password_hash(password, salt []byte) []byte {
	return pbkdf2.Key(password, salt, 4096, 64, sha256.New)
}