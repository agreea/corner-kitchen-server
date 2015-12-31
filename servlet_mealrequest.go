package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"time"
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
	// start worker
	return t
}


//
// 

// curl --data "method=SendRequest&mealId=5&session=c8ac0df2-d17f-4ab3-853a-c91989ddf7d7&seats=1&last4=1234&follow=true" https://yaychakula.com/api/mealrequest
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
	last4_s := r.Form.Get("last4")
	last4, err := strconv.ParseInt(last4_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	count_s := r.Form.Get("seats")
	count, err := strconv.ParseInt(count_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	if r.Form.Get("follow") == "true" {
		RecordFollowHost(t.db, guest.Id, host.Id)
	}
	_, err = GetMealRequestByGuestIdAndMealId(t.db, guest.Id, meal.Id)
	if err != nil { // error here (99%) means the request doesn't exist
		return t.record_request(guest, host, meal, count, last4) // add last 4 as an arg in the request
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
	// get request and make sure it hasn't been responded to already
	request, err := GetMealRequestById(t.db, request_id)
	if err != nil {
		return APIError("Could not locate request", 400)
	}
	if request.Status != 0 {
		return APIError("You already responded to this request.", 400)
	}
	// process, check, and record the response
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
	if err != nil {
		log.Println(err)
		return APIError("Failed to record response.", 400)
	}
	err = t.notify_guest(updated_request)
	if err != nil {
		log.Println(err)
		return APIError("Failed to notify guest", 500)
	}
	return APISuccess(updated_request)
}

func (t *MealRequestServlet) notify_guest(updated_request *MealRequest) (error) {
	guest, err := GetGuestById(t.db, updated_request.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	meal, err := GetMealById(t.db, updated_request.Meal_id)
	if err != nil {
		log.Println(err)
		return err
	}
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return err
	}
	if guest.Phone != "" {
		err := t.text_guest(guest, host, meal, updated_request.Status)
		if err != nil {
			log.Println(err)
		}
		return err
	} 
	err = t.email_guest(guest, host, meal, updated_request.Status)
	if err != nil {
		log.Println(err)
	}
	return err
}

// Called to let them know if they made it
func (t *MealRequestServlet) text_guest(guest *GuestData, host *HostData, meal *Meal, status int64) (error) {
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	msg := new(SMS)
	msg.To = guest.Phone
	// Mon Jan 2 15:04:05 -0700 MST 2006
	// Good news - {HOST} welcomed you to {DINNER}! It's at {ADDRESS} at {TIME}. See you there! :) 
	if status == 1 {
		// get just the hour, convert it to 12 hour format
		msg.Message = fmt.Sprintf("Good news - %s welcomed you to %s! It's at %s at %s. See you there! :)",
			host_as_guest.First_name, 
			meal.Title,
			BuildTime(meal.Starts),
			host.Address)
	} else if status == -1 {
	// Bummer... {HOST} could not welcome you to {DINNER}. I'm sorry :/
		msg.Message = fmt.Sprintf("Bummer... %s could not welcome you to %s! I'm sorry :/",
			host_as_guest.First_name, 
			meal.Title)
	} 
	t.twilio_queue <- msg
	return nil
}

func (t *MealRequestServlet) email_guest(guest *GuestData, host *HostData, meal *Meal, status int64) error {
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return err

	}
	if status == 1 {
		subject := fmt.Sprintf("%s Welcomed You to Their Chakula Meal!", host_as_guest.First_name)
		html :=fmt.Sprintf("<p>Get excited!</p><p>The dinner is at %s, %s</p>" + 
							"<p>Please reply to this email if you need any help.</p>" +
							"<p>View the meal again <a href=https://yaychakula.com/meal.html?Id=%d" + 
							">here</a> " +
							"<p>Peace, love and full stomachs,</p>" +
							"<p>Chakula</p>", 
							host_as_guest.First_name, 
							host.Address, 
							BuildTime(meal.Starts), 
							meal.Id)
		SendEmail(guest.Email, subject, html)
	} else {
		subject := fmt.Sprintf("%s Couldn't Welcome You to their Chakula Meal", host_as_guest.First_name)
		html :=fmt.Sprintf("<p>Bummer.</p><p>There's always hope...</p>" + 
							"<p>Michael Jordan got cut from his high school's JV basketball team." + 
							"<p>His coach probably didn't expect him to ball with Bugs Bunny in" +
							" the 1996 blockbuster, <i>Space Jam</i></p>" +
							"<p>Love,</p><p>Chakula</p>")
		SendEmail(guest.Email, subject, html)
	}
	return nil
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
func (t *MealRequestServlet) record_request(guest *GuestData, host *HostData, meal *Meal, count int64, last4 int64) *ApiResult {
	meal_req := new(MealRequest)
	meal_req.Guest_id = guest.Id
	meal_req.Meal_id = meal.Id
	meal_req.Seats = count
	meal_req.Last4 = last4
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
	guest_as_host, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate host.", 500)
	}
	// Text the host "<session.Guest.Name> wants to join <meal.Title>. Please respond here: https://yaychakula.com/req/<reqId> "
	msg := new(SMS)
	msg.To = guest_as_host.Phone
	msg.Message = fmt.Sprintf("Yo! %s wants to join %s. Please respond: https://yaychakula.com/request.html?Id=%d",
		guest.First_name, meal.Title,
		saved_request.Id)
	t.twilio_queue <- msg
	return APISuccess(meal_req)
}

func BuildTime(ts time.Time) string {
	loc, _ := time.LoadLocation("America/New_York")
	hour_format := "15"
	hour_s := ts.In(loc).Format(hour_format)
	hour, err := strconv.ParseInt(hour_s, 10, 64)
	if err != nil {
		log.Println(err)
		return ""
	}
	var format string
	if hour > 11 {
		format = ":04 PM, Mon Jan 2"
	} else {
		format = ":04 AM, Mon Jan 2"
	}
	var hour_in12 int
	if hour > 12 { // 1 (PM)
		hour_in12 = int(hour - 12)
	} else if hour > 0 { // 11 (AM)
		hour_in12 = int(hour)
	} else {
		hour_in12 = 12 // midnight
	}
	// final time: {hour_in12}:04 {AM/PM}, Mon Jan 2
	readable_time := strconv.Itoa(hour_in12) + ts.In(loc).Format(format)
	return readable_time
}