package main

import (
	"bytes"
	"code.google.com/p/go-uuid/uuid"
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
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

const alphanumerics = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"

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
	fbId := resp["id"]
	fb_id_exists, err := t.fb_id_exists(resp["id"])
	if err != nil {
		return APIError("Could not login", 500)
	}
    if fb_id_exists {
			t.process_login(fbId)
	} else {
		guestData, err := t.create_guest(resp["email"], resp["name"], resp["id"])
		if err != nil {
			return APIError("Failed to create user", 500)
		}
		return APISuccess(guestData)
	}
}

// Returns json data
// Todo: json encoding response body contents
func (t *KitchenUserServlet) get_fb_data_for_token(fbToken string) (map[string]interface{}, err error) {
	resp, err := ExecuteGetForJSON("https://graph.facebook.com/me?fields=id,name,email&access_token="+fbToken)
	if err != nil {
		return nil, err
	} else {
		return resp, nil
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
func (t *KitchenUserServlet) process_login(fbid int) (string, error) {
	guestdata, err := GetGuestByFbId(t.db, fbid)
	if err != nil {
		return "", err
	}
	guest_session, err := t.session_manager.GetGuestSessionById(guestdata.Id)
	if err != nil {
		return "", err
	}
	if guest_session != "" {
		return guest_session, nil
	} else {
		guestdata.Session_token, err = t.session_manager.CreateSessionForGuest(guestdata.Id)
		if err != nil {
			return "", err
		}
		return guestdata.Session_token, nil
	}
}
// Create a new user + session based off of the data returned from facebook and return a GuestData object
func (t *KitchenUserServlet) create_guest(email string, name string, fb_id string) (*GuestData, error) {
	prof_pic_url := "http://graph.facebook.com/" + fbId + "/picture?width=400"
	_, err = t.db.Exec(`INSERT INTO Guest
		(Email, Name, FbId, profpic) VALUES (?, ?, ?, ?)`,
		email, name, fb_id, prof_pic_url)
	guestData := GetGuestByFbId(fb_id)
	guestData.Session_token = t.session_manager.CreateSessionForGuest(guestData.Id)
	if err != nil {
		log.Println("Register", err)
		return nil, err
	}
	return guestData, nil
}

// Check if a fbId already exists in the chakula DB.
// Returns an error if any database operation fails.
func (t *KitchenUserServlet) fb_id_exists(fb_id int) (bool, error) {
	rows, err := t.db.Query("SELECT Id FROM Guest WHERE FbId = ?", fb_id)
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