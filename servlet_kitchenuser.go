package main

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"
	"strconv"
	"strings"
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
func (t *KitchenUserServlet) AddStripe(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	stripe_token := r.Form.Get("stripeToken")
	last4_s := r.Form.Get("last4")
	session_exists, kitchenSession, err := t.session_manager.GetGuestSession(session_id)
	if !session_exists {
		return APIError("Invalid Session", 400)
	}
	last4, err := strconv.ParseInt(meal_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Invalid last 4 digits", 400)
	}
	guestData := kitchenSession.Guest
	err = SaveStripeToken(t.db, stripe_token, last4, guestData)
	if err != nil {
		return APIError("Failed to Save Stripe Data", 500)
	}
	return APISuccess(stripe_token)
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
	if resp.Id == "" {
		return APIError("Error connecting to Facebook", 500)
	}
	fb_id := resp.Id
	name := resp.Name
	email := resp.Email
	subscribe_s := r.Form.Get("subscribe")
	log.Println(subscribe_s)
	if subscribe_s == "true"  {
		log.Println(email)
		MailChimpRegister(email, false, t.db)
	}
	fb_id_exists, err := t.fb_id_exists(fb_id)
	if err != nil {
		return APIError("Could not find user", 500)
	}
	long_token, expires, err := t.get_fb_long_token(fbToken)
	if err != nil || expires == 0 {
		return APIError("Could not connect to Facebook", 500)
	}
	if fb_id_exists {
		// update fb credentials for fb_id
		guestData, err := t.process_login(fb_id, long_token, expires)
		if err != nil {
			return APIError("Could not login", 500)
		}
		return APISuccess(guestData)
	} else {
		// also include long token and expires
		guestData, err := t.create_guest(email, name, fb_id, long_token, expires)
		if err != nil {
			return APIError("Failed to create user", 500)
		}
		return APISuccess(guestData)
	}
}

// Returns json data
// Todo: json encoding response body contents
func (t *KitchenUserServlet) get_fb_data_for_token(fb_token string) (fbresponse *FacebookResp, err error) {
	resp, err := http.Get("https://graph.facebook.com/me?fields=id,name,email&access_token=" + fb_token)
	if err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()
		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println(err)
			return nil, err
		} else {
			err := json.Unmarshal(contents, &fbresponse)
			if err != nil {
				log.Println(err)
				return nil, err
			} else {
				log.Println(fbresponse)
				return fbresponse, nil
			}
		}
	}
}

// get the longterm access token
// store it with the user
// 
func (t *KitchenUserServlet) get_fb_long_token(fb_token string) (long_token string, expires int, err error) {
	resp, err := http.Get("https://graph.facebook.com/oauth/access_token?" +
							"grant_type=fb_exchange_token&client_id=828767043907424" +
							"&client_secret=***REMOVED***&fb_exchange_token=" + fb_token)
	if err != nil {
		return "",0, err
	} else {
		log.Println("FB Token: Received response")
		defer resp.Body.Close()
		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "",0, err
		}
		content_str := string(contents)

		fbResponseSlice := strings.Split(content_str, "&")
		if len(fbResponseSlice) == 1 {
			return "", 0, err
		}
		tokenSlice := strings.Split(fbResponseSlice[0], "=")
		expiresSlice := strings.Split(fbResponseSlice[1], "=")
		long_token := tokenSlice[1]
		expires_s := expiresSlice[1]
		expires64, err := strconv.ParseInt(expires_s, 10, 64)
		if err != nil {
			log.Println(err)
			return long_token, 0, err
		}
		log.Println("Fb Token:" + long_token)
		return long_token, int(expires64), nil
			// if the array is size 1 you have an error
			// else, split the first item by "="
			//			then split the second item by "="
			// try to make the second item in the second item an int
			// the second item in the first item should be your token
			// if err != nil {
			// 	log.Println(err)
			// 	return "",0, err
			// } else {
			// 	// if there's an error in the request:
			// 	if fb_error, error_present := fbHash["error"]; error_present {
			// 		log.Println(fb_error)
			// 		return "", 0, err
			// 	} 
			// 	long_token = fbHash["access_token"]
			// 	log.Println("Successfully got token: " + long_token)
			// 	return long_token, int(expires64), nil
			// }
	}
}

func (t *KitchenUserServlet) Get(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 200)
	}
	return APISuccess(session.Guest)
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
func (t *KitchenUserServlet) process_login(fb_id string, fb_long_token string, expires int) (*GuestData, error) {
	// update FB token
	err := UpdateGuestFbToken(t.db, fb_id, fb_long_token)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	guestData, err := GetGuestByFbId(t.db, fb_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	guest_session, err := t.session_manager.GetGuestSessionById(int64(guestData.Id))
	if err != nil {
		return nil, err
	}
	if guest_session != "" {
		guestData.Session_token = guest_session
		return guestData, nil
	} else {
		guestData.Session_token, err = t.session_manager.CreateSessionForGuest(int64(guestData.Id), expires)
		if err != nil {
			return nil, err
		}
		return guestData, nil
	}
}

// Create a new user + session based off of the data returned from facebook and return a GuestData object
func (t *KitchenUserServlet) create_guest(email string, name string, fb_id string, fb_long_token string, expires int) (*GuestData, error) {
	// update FB token
	_, err := t.db.Exec(`INSERT INTO Guest
		(Email, Name, Facebook_id, Facebook_long_token, Stripe_cust_id) VALUES (?, ?, ?, ?, ?)`,
		email, name, fb_id, fb_long_token, 0)
	guestData, err := GetGuestByFbId(t.db, fb_id)
	if err != nil {
		log.Println("Create guest", err)
		return nil, err
	}
	guestData.Session_token, err = t.session_manager.CreateSessionForGuest(int64(guestData.Id), expires)
	if err != nil {
		log.Println("Create guest", err)
		return nil, err
	}
	return guestData, nil
}

// Check if a fbId already exists in the chakula DB.
// Returns an error if any database operation fails.
func (t *KitchenUserServlet) fb_id_exists(fb_id string) (bool, error) {
	rows, err := t.db.Query("SELECT Id FROM Guest WHERE Facebook_id = ?", fb_id)
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
