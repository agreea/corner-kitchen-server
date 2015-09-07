package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
)

type MealRequestServlet struct {
	db              *sql.DB
	server_config   *Config
	twilio_queue    chan *SMS
	session_manager *SessionManager
}

func NewMealRequestServlet(server_config *Config, session_manager *SessionManager, twilio_queue chan *SMS) *MealRequestServlet {
	t := new(MealRequestServlet)
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

func (t *MealRequestServlet) SendRequest(r *http.Request) *ApiResult {
	meal_id_s := r.Form.Get("mealId")
	meal_id, err := strconv.ParseInt(meal_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed meal ID", 400)
	}
	session_id := r.Form.Get("session")
	guest, host, meal, err := t.get_guest_host_meal(meal_id, session_id)
	if err != nil {
		return APIError("Couldn't process request", 400)
	}
	_, err = GetMealRequestByGuestIdAndMealId(t.db, guest.Id, meal.Id)
	if err != nil { // error here (99%) means the request doesn't exist
		return t.record_request(guest, host, meal)
	}
	return APIError("Meal request already exists", 400)
}

func (t *MealRequestServlet) get_guest_host_meal(meal_id int64, session_id string) (*GuestData, *HostData, *Meal, error) {
	// Get the guest info.
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return nil, nil, nil, err
	}
	if !session_valid {
		return nil, nil, nil, err
	}
	guest := session.Guest

	// get the meal
	meal, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, nil, nil, err
	}
	// return guest, nil, meal, nil
	// get the host
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return nil, nil, nil, err
	}

	return guest, host, meal, nil
}

// Called if the meal request doesn't exist. Generate and save it
func (t *MealRequestServlet) record_request(guest *GuestData, host *HostData, meal *Meal) *ApiResult {
	meal_req := new(MealRequest)
	meal_req.Guest_id = guest.Id
	meal_req.Meal_id = meal.Id
	err := SaveMealRequest(t.db, meal_req)
	if err != nil {
		return APIError("Couldn't record meal request. Please try again", 500)
	}
	saved_request, err := GetMealRequestByGuestIdAndMealId(t.db, guest.Id, meal.Id)
	if err != nil {
		return APIError("Couldn't process meal request. Please try again", 500)
	}
	// Text the host "<session.Guest.Name> wants to join <meal.Title>. Please respond here: https://yaychakula.com/req/<reqId> "
	msg := new(SMS)
	msg.To = host.Phone
	msg.Message = fmt.Sprintf("Yo! %s wants to join %s. Please respond: https://yaychakula.com/req/%d", 
								guest.Name, meal.Title, 
								saved_request.Id)
	t.twilio_queue <- msg
	return APISuccess(meal_req)
}
