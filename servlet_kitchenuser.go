package main

import (
	"code.google.com/p/go.crypto/pbkdf2"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"encoding/base64"
	_ "github.com/go-sql-driver/mysql"
	"code.google.com/p/go-uuid/uuid"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"
	"strconv"
	"strings"
	"fmt"
	"bytes"
)

type KitchenUserServlet struct {
	db              *sql.DB
	random          *rand.Rand
	server_config   *Config
	session_manager *SessionManager
	twilio_queue    chan *SMS

}

func NewKitchenUserServlet(server_config *Config, session_manager *SessionManager, twilio_queue chan *SMS) *KitchenUserServlet {
	t := new(KitchenUserServlet)
	t.random = rand.New(rand.NewSource(time.Now().UnixNano()))
	t.twilio_queue = twilio_queue
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
/*
curl --data "method=AddStripe&session=f1caa66a-3351-48db-bcb3-d76bdc644634&stripeToken=blablabla&last4=1234" https://yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) AddStripe(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	stripe_token := r.Form.Get("stripeToken")
	last4_s := r.Form.Get("last4")
	session_exists, kitchenSession, err := t.session_manager.GetGuestSession(session_id)
	if !session_exists {
		return APIError("Invalid Session", 400)
	}
	last4, err := strconv.ParseInt(last4_s, 10, 64)
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

/*
curl --data "method=GetLast4s&session=f1caa66a-3351-48db-bcb3-d76bdc644634" https://yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) GetLast4s(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_exists, kitchenSession, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not process session", 400)
	}
	if !session_exists {
		return APIError("Session invalid", 400)
	}
	last4s, err := GetLast4sForGuest(t.db, kitchenSession.Guest.Id)
	if err != nil {
		return APIError("Could locate credit cards associated with this account", 400)
	}
	return APISuccess(last4s)
}
/*
curl --data "method=AddPhone&session=f1caa66a-3351-48db-bcb3-d76bdc644634&phone=1234567890" https://qa.yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) AddPhone(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	phone := r.Form.Get("phone")
	session_exists, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate user", 400)
	}
	if !session_exists {
		return APIError("Invalid session", 400)
	}
	err = UpdatePhoneForGuest(t.db, phone, session.Guest.Id)
	return APISuccess("OK")
}

// TODO: 	List management -- unsubscribe previous emails and subscribe the next next emails
// 			Subscribe -- check if the person wants to receive weekly emails (eventually make this by market)
/*
curl --data "method=AddEmail&session=f1caa66a-3351-48db-bcb3-d76bdc644634&email=aka61@georgetown.edu" https://qa.yaychakula.com/api/kitchenuser
*/

func (t *KitchenUserServlet) AddEmail(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	email := r.Form.Get("email")
	// subscribe := r.Form.Get("subscribe")
	// TODO: update all associated stripe customer objects 
	session_exists, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate user", 400)
	}
	if !session_exists {
		return APIError("Invalid session", 400)
	}
	// if session.Guest.Email != nil {
	// 	// unsubscribe old email
	// }
	err = UpdateEmailForGuest(t.db, email, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update email. Please try again", 500)
	}
	MailChimpRegister(email, false, t.db)
	return APISuccess("OK")
}

// Create a login session for a user.
// Session tokens are stored in a local cache, as well as back to the DB to
// support multi-server architecture. A cache miss will result in a DB read.
func (t *KitchenUserServlet) LoginFb(r *http.Request) *ApiResult {
	// if you don't have the fb in your guest table,
	// create a long-lived access token from the short lived one
	fbToken := r.Form.Get("fbToken")
	resp, err := t.get_fb_data_for_token(fbToken)
	if err != nil {
		log.Println(err)
		return APIError("Invalid Facebook Login", 400)
	}
	if resp.Id == "" {
		return APIError("Error connecting to Facebook", 500)
	}
	fb_id_exists, err := t.fb_id_exists(resp.Id)
	if err != nil {
		return APIError("Could not find user", 500)
	}
	long_token, expires, err := t.get_fb_long_token(fbToken)
	if err != nil || expires == 0 {
		log.Println(err)
		return APIError("Could not connect to Facebook", 500)
	}
	if fb_id_exists {
		// update fb credentials for fb_id
		guestData, err := t.process_login_fb(resp.Id, long_token, expires)
		guestData.Facebook_long_token = "You wish :)";
		if err != nil {
			return APIError("Could not login", 500)
		}
		return APISuccess(guestData)
	} else {
		// also include long token and expires
		guestData, err := t.create_guest_fb(resp.First_name, resp.Last_name, resp.Id, long_token, expires)
		if err != nil {
			return APIError("Failed to create user", 500)
		}
		guestData.Facebook_long_token = "You wish :)";
		return APISuccess(guestData)
	}
}

func (t *KitchenUserServlet) LoginEmail(r *http.Request) *ApiResult {
	email := r.Form.Get("email")
	password := r.Form.Get("password")
	guest, err := GetGuestByEmail(t.db, email)
	if err != nil {
		log.Println(err)
		return APIError("Invalid email. Please register this email by creating an account.", 400)
	}
	valid, err := t.verify_password_for_guest(guest.Id, password)
	if err != nil {
		log.Println(err)
		return APIError("Could not authenticate user.", 500)		
	}
	if !valid {
		return APIError("Invalid email or password.", 400)
	}
	token_expires := 60 * 60 * 24 * 60 // 60 days
	guest.Session_token, err = t.session_manager.CreateSessionForGuest(guest.Id, token_expires)
	return APISuccess(guest)
}

func (t *KitchenUserServlet) CreateAccountEmail(r *http.Request) *ApiResult {
	// get first name
	first_name := r.Form.Get("firstName")
	last_name := r.Form.Get("lastName")
	email := r.Form.Get("email")
	// check to make sure you don't have that email already
	guest, err := GetGuestByEmail(t.db, email)
	if guest != nil {
		log.Println(fmt.Sprintf("Guest with email %s already exists", email))
		return APIError("Email account is already registered", 400)
	}
	result, err := 
		t.db.Exec(`INSERT INTO Guest
			(First_name, Last_name, Email)
			VALUES
			(?, ?, ?)`,
			first_name,
			last_name,
			email)
	if err != nil {
		log.Println(err)
		return APIError("Could not create account", 500)
	}
	guest_id, err := result.LastInsertId()
	if err != nil {
		log.Println(err)
		return APIError("Could not create account", 500)
	}
	guest, err = GetGuestById(t.db, guest_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not create account", 500)
	}
	// maybe do a transactional email thing??
	password := r.Form.Get("password")
	err = t.set_password_for_guest(guest.Id, password)
	if err != nil {
		log.Println(err)
		return APIError("Could not create account", 500)
	}
	// if it all works out then send back a session
	token_expires := 60 * 60 * 24 * 60 // 60 days
	guest.Session_token, err = t.session_manager.CreateSessionForGuest(int64(guest_id), token_expires)
	if err != nil {
		log.Println(err)
		return APIError("Could not create account", 500)
	}
	return APISuccess(guest)
}

func (t *KitchenUserServlet) set_password_for_guest(guest_id int64, pass string) error {
	password_salt := t.generate_random_bytestring(64)
	password_hash := t.generate_password_hash([]byte(pass), password_salt)
	_, err := t.db.Exec("UPDATE Guest SET Password_hash = ?, Password_salt = ? WHERE Id = ?",
		base64.StdEncoding.EncodeToString(password_hash),
		base64.StdEncoding.EncodeToString(password_salt),
		guest_id,
	)
	return err
}

// Create a random bytestring
func (t *KitchenUserServlet) generate_random_bytestring(length int) []byte {
	random_bytes := make([]byte, length)
	for i := range random_bytes {
		random_bytes[i] = byte(t.random.Int() & 0xff)
	}
	return random_bytes
}

// Generate a PBKDF password hash. Use 4096 iterations and a 64 byte key.
func (t *KitchenUserServlet) generate_password_hash(password, salt []byte) []byte {
	return pbkdf2.Key(password, salt, 4096, 64, sha256.New)
}

// Verify a password for a username.
// Returns whether or not the password was valid and whether an error occurred.
func (t *KitchenUserServlet) verify_password_for_guest(guest_id int64, pass string) (bool, error) {
	rows, err := t.db.Query("SELECT Password_hash, Password_salt FROM Guest WHERE Id = ?", guest_id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var password_hash_base64 string
	var password_salt_base64 string
	for rows.Next() {
		if err := rows.Scan(&password_hash_base64, &password_salt_base64); err != nil {
			return false, err
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	password_hash, err := base64.StdEncoding.DecodeString(password_hash_base64)
	if err != nil {
		return false, err
	}
	password_salt, err := base64.StdEncoding.DecodeString(password_salt_base64)
	if err != nil {
		return false, err
	}
	generated_hash := t.generate_password_hash([]byte(pass), []byte(password_salt))
	log.Println("Password hash:")	
	log.Println(base64.StdEncoding.EncodeToString(generated_hash))
	log.Println("Password salt:")
	log.Println(base64.StdEncoding.EncodeToString(password_salt))
	// Verify the byte arrays for equality. bytes.Compare returns 0 if the two
	// arrays are equivalent.
	if bytes.Compare(generated_hash, password_hash) == 0 {
		log.Println("Password hash was valid")
		return true, nil
	} else {
		log.Println("Password hash was invalid")
		return false, nil
	}
}

// Returns json data
// Todo: json encoding response body contents
func (t *KitchenUserServlet) get_fb_data_for_token(fb_token string) (fbresponse *FacebookResp, err error) {
	resp, err := http.Get("https://graph.facebook.com/me?fields=id,first_name,last_name,email&access_token=" + fb_token)
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
	}
}
/*
curl --data "method=Get&session=08534f5c-04cd-4d37-9675-b0dc71c0ddaf" https://qa.yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) Get(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	key := r.Form.Get("key")
	if key != "12q4lkjLK99JnfalsmfFDfdkd" {
		session.Guest.Facebook_long_token = "nah b"
	}
	_, err = GetHostByGuestId(t.db, session.Guest.Id)
	if err != nil {
		session.Guest.Is_host = true
	} else {
		session.Guest.Is_host = false
	}
	return APISuccess(session.Guest)
}

/*
curl --data "method=UserFollows&session=08534f5c-04cd-4d37-9675-b0dc71c0ddaf&hostId=42" https://yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) UserFollows(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	host_id_s := r.Form.Get("hostId")
	host_id, err := strconv.ParseInt(host_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed host ID", 400)
	}

	return APISuccess(GetGuestFollowsHost(t.db, session.Guest.Id, host_id))
}

func (t *KitchenUserServlet) Delete(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	private_key := r.Form.Get("key")
	if private_key != "67lk1j2345lkjd4jSAAA=o030924ffdVNLKAasdfnlAJmlj1rln,.as" {
		log.Println("Tried to delete user without key: " + session_id)
		return APIError("Command failed", 500)
	}
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	_, err = t.db.Exec("DELETE FROM User where Id = ?", session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	return APISuccess("OK")
}

func (t *KitchenUserServlet) UpdateProfPic(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	pic := r.Form.Get("pic")
	file_name, err := CreatePicFile(pic)
	if err != nil {
		log.Println(err)
		return APIError("Failed to save picture", 500)
	}
	err = SaveProfPic(t.db, file_name, session.Guest.Id)
	return APISuccess("OK")
}
/*
curl --data "method=UpdateGuest&session=08534f5c-04cd-4d37-9675-b0dc71c0ddaf&bio='testing-testing-testing'&firstName=Agree&lastName=Ahmed" https://yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) UpdateGuest(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	email := r.Form.Get("Email")
	phone := r.Form.Get("Phone")
	bio := r.Form.Get("Bio")
	firstName := r.Form.Get("First_name")
	lastName := r.Form.Get("Last_name")
	err = UpdateGuest(t.db, firstName, lastName, email, phone, bio, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update guest", 500)
	}
	return APISuccess("OK")
}
func (t *KitchenUserServlet) UpdateEmail(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	email := r.Form.Get("email")
	code := uuid.New()
	err = UpdateEmail(t.db, email, code, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not update email", 500)
	}
	html := fmt.Sprintf("<p>Please click the link below to confirm the email you registered with Chakula</p>" +
			"<a href='https://yaychakula.com/#/confirm_email?Id=%d&Code=%s'>Confirm your email</a>",
			session.Guest.Id, code)
	SendEmail(email, "Confirm your Chakula Email", html)
	return APISuccess("OK")
}
func (t *KitchenUserServlet) UpdateBio(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	bio := r.Form.Get("bio")
	err = UpdateBio(t.db, bio, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update bio.", 500)
	}
	return APISuccess("OK")
}
func (t *KitchenUserServlet) VerifyEmail(r *http.Request) *ApiResult {
	guest_id_s := r.Form.Get("Id")
	guest_id, err := strconv.ParseInt(guest_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed Guest Id", 400)
	}
	sent_code := r.Form.Get("Code")
	code, err := GetEmailCodeForGuest(t.db, guest_id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to verify email", 500)
	}
	if code == sent_code {
		err = VerifyEmailForGuest(t.db, guest_id)
		if err != nil {
			log.Println(err)
			return APIError("Failed to verify email", 500)
		}
		return APISuccess("OK")
	}
	return APIError("Incorrect Email.", 400)
}
/*
curl --data "method=UpdatePhone&session=08534f5c-04cd-4d37-9675-b0dc71c0ddaf&phone=4438313923" https://yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) UpdatePhone(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	phone := r.Form.Get("phone")
	generated_pin := t.random.Int()%10000
	if generated_pin < 1000 {
		generated_pin = generated_pin * 10 + t.random.Int() % 10
	}
	err = UpdatePhone(t.db, phone, int64(generated_pin), session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update phone", 500)
	}
	msg := new(SMS)
	msg.To = phone
	msg.Message = fmt.Sprintf("Your Chakula PIN: %d", generated_pin)
	t.twilio_queue <- msg
	return APISuccess("OK")
}
/*
curl --data "method=UserFollows&session=08534f5c-04cd-4d37-9675-b0dc71c0ddaf&pin=1234" https://yaychakula.com/api/kitchenuser
*/
func (t *KitchenUserServlet) VerifyPhone(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Internal Server Error", 500)
	}
	if !session_valid {
		return APIError("Session has expired. Please log in again", 400)
	}
	pin_s := r.Form.Get("pin")
	sent_pin, err := strconv.ParseInt(pin_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed pin", 400)
	}
	pin, err := GetPhonePinForGuest(t.db, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to verify phone", 500)
	}
	if pin == sent_pin {
		err = VerifyPhoneForGuest(t.db, session.Guest.Id)
		if err != nil {
			log.Println(err)
			return APIError("Failed to verify phone", 500)
		}
		return APISuccess("OK")
	}
	return APIError("Incorrect PIN.", 400)
}
// Fetches a user's data and creates a session for them.
// Returns a pointer to the userdata and an error.
func (t *KitchenUserServlet) process_login_fb(fb_id string, fb_long_token string, expires int) (*GuestData, error) {
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
func (t *KitchenUserServlet) create_guest_fb(first_name string, last_name string, fb_id string, fb_long_token string, expires int) (*GuestData, error) {
	// update FB token
	_, err := t.db.Exec(`INSERT INTO Guest
		(First_name, Last_name, Facebook_id, Facebook_long_token) VALUES (?, ?, ?, ?)`,
		first_name, last_name, fb_id, fb_long_token)
	if err != nil {
		log.Println("Create guest", err)
	}
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