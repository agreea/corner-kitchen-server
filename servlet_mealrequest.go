package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	go t.process_popup_charge_worker()
	return t
}

/* 
curl --data "method=BookPopup&session=f1caa66a-3351-48db-bcb3-d76bdc644634&popupId=15&seats=2&last4=1234" https://qa.yaychakula.com/api/meal
*/
func (t *MealRequestServlet) BookPopup(r *http.Request) *ApiResult{
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
	// record a host-guest follow
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
	if err == sql.ErrNoRows {
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
	meal_price, err := GetMealPriceById(t.db, popup.Meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not calculate meal price", 500)
	}	
	booking.Meal_price = meal_price 
	err = SavePopupBooking(t.db, booking)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't record booking. Please try again", 500)
	}
	saved_booking, err := GetBookingByGuestAndPopupId(t.db, guest.Id, popup.Id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't process booking. Please try again", 500)
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
	meal_price, err := GetMealPrice(t.db, meal)
	if err != nil {
		log.Println(err)
		return err
	}
	subject := fmt.Sprintf("%s Welcomed You to %s!", host_as_guest.First_name, meal.Title)
	html :=fmt.Sprintf("<p>Get excited!</p><p>The dinner is at %s, %s, %s, %s</p>" + 
		"<p>Please reply to this email if you need any help.</p>" +
		"<p>View the meal again <a href=https://yaychakula.com/meal/%d" + 
		">here</a></p>" +
		"<p>Your card will be charged $%.2f.</p>" + 
		"<p>Peace, love and full stomachs,</p>" +
		"<p>Chakula</p>", 
		BuildTime(popup.Starts), 
		popup.Address, 
		popup.City,
		popup.State, 
		meal.Id,
		meal_price)
	SendEmail(guest_email, subject, html)
	return nil
}

func (t *MealRequestServlet) process_popup_charge_worker() {
	// get all meals that happened 7 - 8 days ago
	for {
		t.process_meal_charges()
		time.Sleep(time.Hour)
	}
}

// TODO: handle failed charges...

func (t *MealRequestServlet) process_meal_charges(){
	if server_config.Version.V != "prod" { // only run this routine on prod
		log.Println("Exiting meal_charge routine on qa")
		return
	}
	window_starts := time.Now().Add(-time.Hour * 3)
	window_ends := time.Now().Add(-time.Hour * 4)
	popups, err := GetPopupsFromTimeWindow(t.db, window_starts, window_ends)
	if err != nil {
		log.Println(err)
		return
	}
	for _, popup := range popups {
		if (popup.Processed == 1) { // skip the processed meals
			continue
		}
		bookings, err := GetBookingsForPopup(t.db, popup.Id)
		if err != nil {
			log.Println(err)
		}
		t.process_bookings(bookings)
		SetPopupProcessed(t.db, popup.Id)
		t.notify_host_payment_processed(popup)
	}
}

func (t *MealRequestServlet) notify_host_payment_processed(popup *Popup) {
	meal, err := GetMealById(t.db, popup.Meal_id)
	if err != nil {
		log.Println(err)
		return
	}
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return
	}
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return
	}
	host_as_guest.Email, err = GetEmailForGuest(t.db, host_as_guest.Id)
	if err != nil {
		log.Println(err)
		return
	}
	subject := "Processed: " + meal.Title
	html := "<p>Chakula processed the payments for the meal you held at " + BuildTime(popup.Starts) + ".</p>" +
			"<p>Please be advised that <strong>Stripe still has to clear the payments</strong> before the funds are transferred to your account." +
			"This should take no more than 4 business days</p>" +
			"<p>To check the status of your funds please log into your <a href='https://stripe.com'>stripe account</a></p>" +
			"<p>If you have any further questions, contact Agree at agree@yaychakula.com</p>" +
			"<p>Sincerely,</p>" +
			"<p>Chakula</p>"
	SendEmail(host_as_guest.Email, subject, html)
}

func (t *MealRequestServlet) process_bookings(bookings []*PopupBooking) {
	for _, booking := range bookings {
		// create stripe charge
		t.charge_booking(booking)
	}
}

/*
curl https://api.stripe.com/v1/charges \
   -u ***REMOVED***: \
   -d amount=___ \
   -d currency=usd \
   -d customer=___ \
   -d destination=___ \
   -d application_fee=___
*/

type StripeCharge struct {
	Amount 			int `json:"amount"`
	Currency   		string `json:"currency"`
	Customer 		string `json:"customer"`
	Host_acct		string `json:"destination"`
	Chakula_fee		int `json:"application_fee"`
}

/* 
curl --data "method=ChargeBooking&id=63&key=***REMOVED***" https://qa.yaychakula.com/api/mealrequest
*/
func (t *MealRequestServlet) ChargeBooking(r *http.Request) *ApiResult {
	booking_id_s := r.Form.Get("id")
	booking_id, err := strconv.ParseInt(booking_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Ya fucked up", 400)
	}

	key := r.Form.Get("key")
	if key != "***REMOVED***" {
		return APIError("Error", 400)
	}

	booking, err := GetBookingById(t.db, booking_id)
	if err != nil {
		log.Println(err)
		return APIError("Fuck", 500)
	}
	t.charge_booking(booking)
	return APISuccess("OKEN")
}

/*
curl https://api.stripe.com/v1/charges \
   -u ***REMOVED***: \
   -d amount=___ \
   -d currency=usd \
   -d customer=___ \
   -d destination=___ \
   -d application_fee=___
*/

func (t *MealRequestServlet) charge_booking(booking *PopupBooking) {
	// Get customer object, meal (to get the price), and host (to get stripe destination)
	if booking.Last4 == 0 { // skip dummy and complementary bookings
		return
	}
	customer, err := GetStripeTokenByGuestIdAndLast4(t.db, booking.Guest_id, booking.Last4)
	if err != nil {
		log.Println(err)
		return
	}
	popup, err := GetPopupById(t.db, booking.Popup_id)
	if err != nil {
		log.Println(err)
		return	
	}
	meal, err := GetMealById(t.db, popup.Meal_id)
	if err != nil {
		log.Println(err)
		return
	}
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return
	}
	host_price_pennies := meal.Price * 100
	seats := float64(booking.Seats)
	total_pennies := int(booking.Meal_price * seats * 100)
	chakula_fee_pennies := total_pennies - int(host_price_pennies * seats)
	log.Println("Price in pennies: %d", host_price_pennies)
	log.Println("Total in pennies: %d", total_pennies)
	log.Println("Chakula fee in pennies: %d", chakula_fee_pennies)
	PostStripeCharge(total_pennies, chakula_fee_pennies, customer.Stripe_token, host.Stripe_user_id)
}

func PostStripeCharge(total, chakula_fee int, customer_token, host_account string) {
	client := &http.Client{}
   	stripe_body := url.Values{
		"amount": {strconv.Itoa(total)},
		"currency": {"usd"},
		"customer": {customer_token},
		"destination": {host_account},
		"application_fee": {strconv.Itoa(chakula_fee)},
	}
	req, err := http.NewRequest(
		"POST",
		"https://api.stripe.com/v1/charges",
		strings.NewReader(stripe_body.Encode()))
	if err != nil {
		log.Println(err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if server_config.Version.V == "prod" {
		req.SetBasicAuth("***REMOVED***:", "")
	} else {
		req.SetBasicAuth("***REMOVED***:", "")
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(resp)
	// TODO: react according to Stripe response!
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