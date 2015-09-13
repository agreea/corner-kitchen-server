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

type MealRequestRead struct {
	Guest_name string
	Guest_pic  string
	Meal_title string
	Status     int64
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
	if guest == nil {
		return APIError("Couldn't process guest", 400)
	}
	if meal == nil {
		return APIError("Couldn't process meal", 400)
	}
	if host == nil {
		return APIError("Couldn't process host", 400)
	}
	if err != nil {
		return APIError("Couldn't process request", 400)
	}
	_, err = GetMealRequestByGuestIdAndMealId(t.db, guest.Id, meal.Id)
	if err != nil { // error here (99%) means the request doesn't exist
		// get last4 digits
		// convert digits to int
		return t.record_request(guest, host, meal) // add last 4 as an arg in the request
	}
	return APIError("You've already requested to join this meal", 400)
}

func (t *MealRequestServlet) GetRequest(r *http.Request) *ApiResult {
	log.Println("=====Getting Request======")
	request_id_s := r.Form.Get("requestId")
	request_id, err := strconv.ParseInt(request_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed meal ID", 400)
	}
	// get the request by its id
	request, err := GetMealRequestById(t.db, request_id)
	if err != nil {
		return APIError("Could not locate request", 400)
	}
	// get the guest data by their id
	guest, err := GetGuestById(t.db, request.Guest_id)
	if err != nil {
		return APIError("Could not locate guest", 500)
	}
	request_read := new(MealRequestRead)
	request_read.Guest_name = guest.First_name
	request_read.Guest_pic = GetFacebookPic(guest.Facebook_id)
	request_read.Status = request.Status
	meal, err := GetMealById(t.db, request.Meal_id)
	if err != nil {
		return APIError("Could not locate meal", 500)
	}
	request_read.Meal_title = meal.Title
	return APISuccess(request_read)
}

func (t *MealRequestServlet) Respond(r *http.Request) *ApiResult {
	request_id_s := r.Form.Get("requestId")
	request_id, err := strconv.ParseInt(request_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed request ID", 400)
	}
	request, err := GetMealRequestById(t.db, request_id)
	if err != nil {
		return APIError("Could not locate request", 400)
	}
	if request.Status != 0 {
		return APIError("You already responded to this request.", 400)
	}

	response_s := r.Form.Get("response")
	response, err := strconv.ParseInt(response_s, 10, 64)
	if response != 1 && response != -1 {
		return APIError("Invalid response.", 400)
	}
	err = UpdateMealRequest(t.db, request_id, response)
	if err != nil {
		return APIError("Failed to record response.", 400)
	}
	updated_request, err := GetMealRequestById(t.db, request_id)
	// 	// TODO:
	// get guest by id updated_request.Guest_id
	// if guest.Phone != "" or 0 or w/e, text them the result.
	if err != nil {
		return APIError("Failed to record response.", 400)
	}
	return APISuccess(updated_request)
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
		return guest, nil, nil, err
	}
	// get the host
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return guest, nil, meal, err
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
		log.Println(err)
		return APIError("Couldn't record meal request. Please try again", 500)
	}
	saved_request, err := GetMealRequestByGuestIdAndMealId(t.db, guest.Id, meal.Id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't process meal request. Please try again", 500)
	}
	// Text the host "<session.Guest.Name> wants to join <meal.Title>. Please respond here: https://yaychakula.com/req/<reqId> "
	msg := new(SMS)
	msg.To = host.Phone
	msg.Message = fmt.Sprintf("Yo! %s wants to join %s. Please respond: https://yaychakula.com/request.html?Id=%d",
		guest.First_name, meal.Title,
		saved_request.Id)
	t.twilio_queue <- msg
	return APISuccess(meal_req)
}
