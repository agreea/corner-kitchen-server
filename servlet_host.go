package main

import (
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

type HostServlet struct {
	db              *sql.DB
	server_config   *Config
	twilio_queue    chan *SMS
	session_manager *SessionManager
}

type HostResponse struct {
	First_name 		string
	Last_name 		string
	Email 			string
	Phone 			string
	Address 		string
	Stripe_connect	bool
}

func NewHostServlet(server_config *Config, session_manager *SessionManager, twilio_queue chan *SMS) *HostServlet {
	t := new(HostServlet)
	t.server_config = server_config
	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("HostServlet", "Failed to open database:", err)
	}
	t.db = db
	t.session_manager = session_manager
	t.twilio_queue = twilio_queue
	return t
}
/*
https://connect.stripe.com/oauth/authorize?response_type=code&client_id=ca_6n8She3UUNpFgbv1sYtB28b6Db7sTLY6&scope=read_write
curl --data "method=StripeConnect&session=c12c1704-d2b0-4af5-83eb-a562afcfe277&auth=ac_6ygyqZ4QBFVNl5s7z7VgEVULAMFaNoT7"  https://yaychakula.com/api/host
*/
func (t *HostServlet) StripeConnect(r *http.Request) *ApiResult {
	log.Println("=======Stripe Connect called======")
	auth := r.Form.Get("auth")
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil || valid == false || session.Guest == nil {
		return APIError("Invalid session", 500)
	}
	guest := session.Guest
	
	host, err := GetHostByGuestId(t.db, guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 500)
	}
	stripeResponse, err := t.stripe_auth(auth)
	if err != nil {
		log.Println(err)
		return nil
	}
	if stripe_error, error_present := stripeResponse["error"]; error_present {
		log.Println(stripe_error)
		return APIError(stripe_error.(string)+stripeResponse["error_description"].(string), 400)
	}
	if stripeResponse["livemode"].(bool) == false {
		return APIError("Stripe misconfiguration.", 500)
	}
	err = UpdateStripeConnect(t.db, stripeResponse, host.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not update Stripe Data. Please try again.", 500)
	}
	log.Println(stripeResponse["stripe_user_id"].(string))
	log.Println(stripeResponse)
	return APISuccess(nil)
}

func (t *HostServlet) UpdateHost(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil || valid == false || session.Guest == nil {
		return APIError("Invalid session", 500)
	}
	host_as_guest := session.Guest
	host, err := GetHostByGuestId(t.db, host_as_guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 500)
	}
	first_name := r.Form.Get("firstName")
	last_name := r.Form.Get("lastName")
	email := r.Form.Get("email")
	phone := r.Form.Get("phone")
	err = UpdateGuest(t.db, first_name, last_name, email, phone, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update host data", 500)
	}
	address := r.Form.Get("address")
	err = UpdateHost(t.db, address, host.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update host data", 500)
	}
	return APISuccess(nil)
}

func (t *HostServlet) stripe_auth(auth string) (map[string]interface{}, error) {
	resp, err := http.PostForm("https://connect.stripe.com/oauth/token",
		url.Values{"client_secret": {"***REMOVED***"},
			"code": {auth},
			"grant_type": {"authorization_code"},
		})
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	stripeJSON := make(map[string]interface{})
	err = json.Unmarshal(contents, &stripeJSON)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	return stripeJSON, nil
}

func (t *HostServlet) GetHost(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	guest, host, err := t.get_guest_and_host(session_id)
	host_resp := new(HostResponse)
	if err != nil {
		log.Println(err)
		return APIError("Invalid session", 500)
	}
	if guest == nil { // you had an actual session but it was old
		log.Println("Expired session")
		return APIError("Expired session", 500)
	}
	host_resp.First_name = guest.First_name
	host_resp.Last_name = guest.Last_name
	host_resp.Email = guest.Email
	host_resp.Phone = guest.Phone
	if host == nil { 
		// host hasn't been created yet.
		// Give em what we got on the guest and then create a host object when they updateHost
		host_resp.Stripe_connect = false
		return APISuccess(host_resp)
	}
	host_resp.Address = host.Address
	host_resp.Stripe_connect = !(host.Stripe_user_id == "")
	return APISuccess(host_resp)
}

func (t *HostServlet) get_guest_and_host(session_id string) (*GuestData, *HostData, error) {
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		return nil, nil, err
	}
	if !valid {
		return nil, nil, nil
	}
	host_as_guest := session.Guest
	host, err := GetHostByGuestId(t.db, host_as_guest.Id)
	if err != nil {
		return host_as_guest, nil, err
	}
	return host_as_guest, host, nil
}
// func (t *HostServlet) Pay(r *http.Request) *ApiResult {
// get the guest's Stripe id
// get the meal data
// get the host's Stripe id
// charge the guest for the meal, taking off 22.9% + 30 cents
// get the success response? send success response back
// }
