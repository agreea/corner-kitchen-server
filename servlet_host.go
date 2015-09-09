package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"encoding/json"
	"io/ioutil"
	"net/url"
)

type HostServlet struct {
	db              *sql.DB
	server_config   *Config
	twilio_queue    chan *SMS
	session_manager *SessionManager
}


func NewHostServlet(server_config *Config, session_manager *SessionManager, twilio_queue chan *SMS) *HostServlet {
	t := new(HostServlet)
	t.server_config = server_config
	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewMealRequestServlet", "Failed to open database:", err)
	}
	t.db = db
	t.session_manager = session_manager
	t.twilio_queue = twilio_queue
	return t
}

func (t *HostServlet) StripeConnect(r *http.Request) *ApiResult {
	auth := r.Form.Get("auth")
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil || valid == false {
		return APIError("Invalid session", 500)
	}
	guest := session.Guest
	host, err := GetHostByGuestId(t.db, guest.Id)
	stripeResponse, err := t.stripe_auth(auth)
	if (err != nil || stripeResponse["error"] != nil) {
		return APIError("Could not connect to Stripe", 500)
	}
	host.Id = 3
	return APISuccess(stripeResponse)
	// get the authorization code done
	// get the session done
	// get the guest by the session done
	// get the host by the guest_id done
	// make a post
	/* 
	curl https://connect.stripe.com/oauth/token \
	   -d client_secret=***REMOVED*** \
	   -d code=AUTHORIZATION_CODE \
	   -d grant_type=authorization_code
	*/
	// get the response back
	   // get the stripe_user_id, refresh_token, and access_token and store them in your table
	   // store stripe_row in the host table
	   // return APIResult(Guest)?
}


func (t *HostServlet) stripe_auth(auth string) (map[string]interface{}, error) {
	resp, err := http.PostForm("https://connect.stripe.com/oauth/token", 
		url.Values{"client_secret": {"***REMOVED***"}, 
					"code": {auth},
					"grant_type": {"authorization_code"},
					})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	var stripeJSON map[string]interface{}
	if err != nil {
		return nil, err
	} else {
		err := json.Unmarshal(contents, &stripeJSON)
		if err != nil{
			log.Println("Uh oh")
			return nil, err
		} else {
			return stripeJSON, nil
		}
	}
}
// func (t *HostServlet) Pay(r *http.Request) *ApiResult {
	// get the guest's Stripe id
	// get the meal data
	// get the host's Stripe id
	// charge the guest for the meal, taking off 22.9% + 30 cents
 	// get the success response? send success response back
// }
