package main

import (
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"fmt"
	"strconv"
)

type HostServlet struct {
	db              *sql.DB
	server_config   *Config
	twilio_queue    chan *SMS
	session_manager *SessionManager
}

type HostResponse struct {
	Email 			string
	Phone 			string
	Address 		string
	City 			string
	State 			string
	Prof_pic		string
	Bio 			string
	Stripe_url 		string
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

curl https://api.stripe.com/v1/charges \
   -u ***REMOVED***: \
   -d amount=___ \
   -d currency=usd \
   -d customer=___ \
   -d destination=___ \
   -d application_fee=___

*/
func (t *HostServlet) StripeConnect(r *http.Request) *ApiResult {
	log.Println("=======Stripe Connect called======")
	auth := r.Form.Get("auth")
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate guest", 400)
	}
	if !valid {
		return APIError("Invalid session", 400)
	}
	guest := session.Guest
	host, err := GetHostByGuestId(t.db, guest.Id)
	// create a host if there isn't one and then update their data
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host. Make sure host exists", 400)
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
		log.Println("Stripe wasn't in live mode")
		return APIError("Stripe misconfiguration.", 500)
	}
	err = UpdateStripeConnect(t.db, stripeResponse, host.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not update Stripe Data. Please try again.", 500)
	}
	log.Println(stripeResponse)
	return APISuccess(nil)
}

func (t *HostServlet) Get(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate guest", 400)
	}
	if !valid {
		return APIError("Invalid session", 400)
	}

	host, err := GetHostByGuestId(t.db, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("No host in db matching this record", 400)
	}
	if host.Stripe_user_id != "" {
		host.Stripe_user_id = "yes"
	}
	host.Stripe_access_token = ""
	host.Stripe_refresh_token = ""
	return APISuccess(host)
}

func (t *HostServlet) UpdateHost(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate guest", 400)
	}
	if !valid {
		return APIError("Invalid session", 400)
	}
	guest := session.Guest
	host, err := GetHostByGuestId(t.db, guest.Id)
	// create a host if there isn't one and then update their data
	if err != nil {
		log.Println(err)
		err = CreateHost(t.db, guest.Id)
		if err != nil {
			return APIError("Failed to create host", 500)
		}
		_, err := GetHostByGuestId(t.db, guest.Id)
		if err != nil {
			return APIError("Failed to locate host", 500)
		}
	}
	address := r.Form.Get("address")
	state := r.Form.Get("state")
	city := r.Form.Get("city")
	err = UpdateHost(t.db, address, city, state, host.Id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to update host data", 500)
	}
	return t.GetHost(r)
}

type HostProfile struct {
	Name 			string
	City 			string
	State 			string
	Prof_pic		string
	Bio 			string
	Meals 			[]*Meal
	Follows 		bool
}

func (t *HostServlet) GetProfile(r *http.Request) *ApiResult {
	// new HostProfile
	host_prof := new(HostProfile)
	host_id_s := r.Form.Get("hostId")
	host_id, err := strconv.ParseInt(host_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed host ID", 400)
	}
	host, err := GetHostById(t.db, host_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid host id", 400)
	}
	session_id := r.Form.Get("session")
	if (session_id == "") {
		host_prof.Follows = false
	}
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if (err != nil || !valid) {
		host_prof.Follows = false	
	} else {
		host_prof.Follows = GetGuestFollowsHost(t.db, session.Guest.Id, host_id)
	}
	host_prof.City = host.City
	host_prof.State = host.State
	host_as_guest, err := GetGuestByHostId(t.db, host_id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to locate host", 500)
	}
	host_prof.Name = host_as_guest.First_name
	host_prof.Bio = host_as_guest.Bio
	if host_as_guest.Prof_pic != "" {
		host_prof.Prof_pic = "https://yaychakula.com/" + host_as_guest.Prof_pic
	} else if host_as_guest.Facebook_id != "" {
		host_prof.Prof_pic = GetFacebookPic(host_as_guest.Facebook_id)
	}
	// else: set default
	host_prof.Meals, err = GetMealsForHost(t.db, host_id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to get meals for host", 500)
	}
	for _, meal := range host_prof.Meals {
		meal.Pics, err = GetMealPics(t.db, meal.Id)
		if err != nil {
			log.Println(err)
			continue
		}
	}
	return APISuccess(host_prof)
	// get host id. use to get:
	// guest object (bio, profile pic, etc)
	// past meals
	// upcoming meals
	// if there isn't one, set follows to false
	// else check if the user follows the chef and set that accordingly
	// play it as such
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
/*
curl --data "method=GetHost&session=c12c1704-d2b0-4af5-83eb-a562afcfe277"  https://qa.yaychakula.com/api/host
*/
func (t *HostServlet) GetHost(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate guest", 400)
	}
	if !valid {
		return APIError("Invalid session", 400)
	}
	host_as_guest := session.Guest
	host_as_guest.Email, err = GetEmailForGuest(t.db, host_as_guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate your email. " + 
			"Please make sure you have registered an email with " +
			"Chakula before creating your host account", 400)
	}
	host_as_guest.Phone, err = GetPhoneForGuest(t.db, host_as_guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate your phone number. " + 
			"Please make sure you have registered a phone number with " +
			"Chakula before creating your host account", 400)
	}
	host_resp := new(HostResponse)
	host_resp.Email = host_as_guest.Email
	host_resp.Phone = host_as_guest.Phone
	host_resp.Prof_pic = GetFacebookPic(host_as_guest.Facebook_id)
	host, err := GetHostByGuestId(t.db, host_as_guest.Id)
	if err != nil { // there wasn't a host with this guest id
		log.Println(err)
		err = CreateHost(t.db, host_as_guest.Id)
		if err != nil {
			log.Println(err)
			return APIError("Failed to create Host", 500)
		}
		host_resp.Stripe_connect = false
		return APISuccess(host_resp)
	}
	host_resp.Address = host.Address
	host_resp.City = host.City
	host_resp.State = host.State
	host_resp.Bio = host.Bio
	host_resp.Stripe_connect = !(host.Stripe_user_id == "")
	host_resp.Stripe_url = fmt.Sprintf("https://connect.stripe.com/oauth/authorize?response_type=code&amp;" + 
        "client_id=ca_6n8She3UUNpFgbv1sYtB28b6Db7sTLY6&amp;scope=read_write&amp;" +
        "stripe_user[email]=%s&amp;" +
    	"stripe_user[url]=https://yaychakula.com/host?Id=%d&amp;" +
        "stripe_user[business_name]=%s_on_Chakula&amp;" +
        "stripe_user[business_type]=sole_prop&amp;" +
        "stripe_user[phone_number]=%s&amp;" +
        "stripe_user[first_name]=%s&amp;" +
        "stripe_user[last_name]=%s&amp;" +
        "stripe_user[street_address]=%s&amp;" +
        "stripe_user[city]=%s&amp;" +
        "stripe_user[state]=%s&amp;" +
        "stripe_user[product_category]=food_and_restaurants&amp;" +
        "stripe_user[product_description]=Food&amp;" +
        "stripe_user[country]=US&amp;" +
		"stripe_user[currency]=usd",
		host_as_guest.Email,
		host_as_guest.Id,
		host_as_guest.First_name,
		host_as_guest.Phone,
		host_as_guest.First_name,
		host_as_guest.Last_name,
		host.Address,
		host.City,
		host.State)
	return APISuccess(host_resp)
}
// func (t *HostServlet) Pay(r *http.Request) *ApiResult {
// get the guest's Stripe id
// get the meal data
// get the host's Stripe id
// charge the guest for the meal, taking off 22.9% + 30 cents
// get the success response? send success response back
// }
