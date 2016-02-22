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

// curl --data "method=SendRequest&mealId=70&session=cf31660b-1554-4547-959a-3ef67d834076&seats=1&last4=5154&follow=true" https://yaychakula.com/api/mealrequest
// func (t *MealRequestServlet) SendRequest(r *http.Request) *ApiResult {
// 	meal_id_s := r.Form.Get("mealId")
// 	meal_id, err := strconv.ParseInt(meal_id_s, 10, 64)
// 	if err != nil {
// 		return APIError("Malformed meal ID", 400)
// 	}
// 	session_id := r.Form.Get("session")
// 	guest, host, meal, err := t.get_guest_host_meal(meal_id, session_id)
// 	if guest == nil {
// 		return APIError("Couldn't process guest", 400)
// 	}
// 	if meal == nil {
// 		return APIError("Couldn't process meal", 400)
// 	}
// 	if host == nil {
// 		return APIError("Couldn't process host", 400)
// 	}
// 	if err != nil {
// 		return APIError("Couldn't process request", 400)
// 	}
// 	last4_s := r.Form.Get("last4")
// 	last4, err := strconv.ParseInt(last4_s, 10, 64)
// 	if err != nil {
// 		log.Println(err)
// 		return APIError("Malformed meal ID", 400)
// 	}
// 	count_s := r.Form.Get("seats")
// 	count, err := strconv.ParseInt(count_s, 10, 64)
// 	if err != nil {
// 		log.Println(err)
// 		return APIError("Malformed meal ID", 400)
// 	}
// 	if r.Form.Get("follow") == "true" {
// 		RecordFollowHost(t.db, guest.Id, host.Id)
// 	}
// 	_, err = GetMealRequestByGuestIdAndMealId(t.db, guest.Id, meal.Id)
// 	if err == sql.ErrNoRows { // error here (99%) means the request doesn't exist
// 		return t.record_request(guest, host, meal, count, last4) // add last 4 as an arg in the request
// 	}
// 	return APIError("You've already requested to join this meal", 400)
// }

/* 
curl --data "method=BookPopup&session=f1caa66a-3351-48db-bcb3-d76bdc644634&popupId=15&seats=2&last4=1234" https://qa.yaychakula.com/api/meal
*/
func (t *MealRequestServlet) BookPopup(r *http.Request) *ApiResult{
	// get popup
	// get guest
	// get last 4 digits of cc
	// get seats
	// reserve popup
	popup_id_s := r.Form.Get("popupId")
	popup_id, err := strconv.ParseInt(popup_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed popup ID", 400)
	}
	popup, err := GetPopupById(t.db, popup_id)
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil || !session_valid {
		return APIError("Couldn't process request", 400)
	}
	last4_s := r.Form.Get("last4")
	last4, err := strconv.ParseInt(last4_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed last 4", 400)
	}
	count_s := r.Form.Get("seats")
	count, err := strconv.ParseInt(count_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	if r.Form.Get("follow") == "true" {
		meal, err := GetMealById(t.db, popup.Meal_id)
		if err != nil {
			log.Println(err)
			return APIError("Could not process follow", 500)
		}
		host, err := GetHostById(t.db, meal.Host_id)
		if err != nil {
			log.Println(err)
			return APIError("Could not process follow", 500)
		}
		RecordFollowHost(t.db, session.Guest.Id, host.Id)
	}
	_, err = GetBookingByGuestAndPopupId(t.db, session.Guest.Id, popup_id)
	if err != nil { // error here (99%) means the request doesn't exist
		return t.record_booking(session.Guest, popup, count, last4)
	}
	return APIError("You've already requested to join this meal", 400)
}

func (t *MealRequestServlet) record_booking(guest *GuestData, popup *Popup, count, last4 int64) *ApiResult {
	booking := new(PopupBooking)
	booking.Guest_id = guest.Id
	booking.Popup_id = popup.Id
	booking.Seats = count
	booking.Last4 = last4
	err := SavePopupBooking(t.db, booking)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't record meal request. Please try again", 500)
	}
	saved_booking, err := GetBookingByGuestAndPopupId(t.db, guest.Id, popup.Id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't process meal request. Please try again", 500)
	}
	// Notifies guest that they're attending the meal, with all relevant info
	err = t.notify_guest(saved_booking)
	if err != nil {
		log.Println(err)
		return APIError("Failed to notify guest", 500)
	}
	return APISuccess(saved_booking)
}

func (t *MealRequestServlet) notify_guest(booking *PopupBooking) (error) {
	guest, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	phone, err := GetPhoneForGuest(t.db, guest.Id)
	if err != nil {
		log.Println(err)
	}
	if phone != "" {
		err := t.text_guest(phone, booking)
		if err != nil {
			return err
		}
	} 
	err = t.email_guest(booking)
	return err
}

// Called to let them know if they made it
func (t *MealRequestServlet) text_guest(phone string, booking *PopupBooking) (error) {
	popup, err := GetPopupById(t.db, booking.Popup_id)
	meal, err := GetMealById(t.db, popup.Meal_id)
	if err != nil {
		log.Println(err)
		return err
	}
	host, err := GetHostById(t.db, meal.Host_id)
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	msg := new(SMS)
	msg.To = phone
	// Good news - {HOST} welcomed you to {DINNER}! It's at {ADDRESS} at {TIME}. See you there! :) 
	// get just the hour, convert it to 12 hour format
	msg.Message = fmt.Sprintf("Good news - %s welcomed you to %s! It's at %s at %s, %s, %s. See you there! :)",
		host_as_guest.First_name, 
		meal.Title,
		BuildTime(popup.Starts),
		popup.Address,
		popup.City,
		popup.State)
	t.twilio_queue <- msg
	return nil
}

func (t *MealRequestServlet) email_guest(booking *PopupBooking) error {
	popup, err := GetPopupById(t.db, booking.Popup_id)
	meal, err := GetMealById(t.db, popup.Meal_id)
	if err != nil {
		log.Println(err)
		return err
	}
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return err
	}
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	guest, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	guest_email, err := GetEmailForGuest(t.db, guest.Id)
	if err != nil {
		log.Println(err)
		return err
	}
	subject := fmt.Sprintf("%s Welcomed You to Their Chakula Meal!", host_as_guest.First_name)
	html :=fmt.Sprintf("<p>Get excited!</p><p>The dinner is at %s, %s, %s, %s</p>" + 
		"<p>Please reply to this email if you need any help.</p>" +
		"<p>View the meal again <a href=https://yaychakula.com/meal/%d" + 
		">here</a> " +
		"<p>Peace, love and full stomachs,</p>" +
		"<p>Chakula</p>", 
		BuildTime(popup.Starts), 
		popup.Address, 
		popup.City,
		popup.State, 
		meal.Id)
	SendEmail(guest_email, subject, html)
	return nil
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