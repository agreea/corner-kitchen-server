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
	"errors"
    "github.com/sendgrid/sendgrid-go"
    "io/ioutil"
)

type MealRequestServlet struct {
	db              *sql.DB
	server_config   *Config
	twilio_queue    chan *SMS
	session_manager *SessionManager
	sg_client 		*sendgrid.SGClient // sendGrid client
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
	t.sg_client = sendgrid.NewSendGridClient(server_config.SendGrid.User, server_config.SendGrid.Pass)

	// start worker
	// go t.process_popup_charge_worker()
	return t
}

/* 
curl --data "method=BookPopup&session=f1caa66a-3351-48db-bcb3-d76bdc644634&popupId=36&seats=2&last4=4242" https://qa.yaychakula.com/api/meal
*/
func (t *MealRequestServlet) BookPopup(r *http.Request) *ApiResult{
	popup_id_s := r.Form.Get("popupId")
	popup_id, err := strconv.ParseInt(popup_id_s, 10, 64)
	if err != nil {
		return APIError("Malformed popup ID", 400)
	}
	popup, err := GetPopupById(t.db, popup_id)
	session_id := r.Form.Get("session")
	session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
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
		if followsHost := GetGuestFollowsHost(t.db, session.Guest.Id, meal.Host_id); !followsHost {
			RecordFollowHost(t.db, session.Guest.Id, meal.Host_id)
		}
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
	if err = t.send_guest_confirmation(saved_booking); err != nil {
		log.Println(err)
		return APIError("Failed to notify guest", 500)
	}
	if err = t.notify_host_booking(saved_booking); err != nil {
		log.Println(err)
		return APIError("Failed to notify guest", 500)
	}
	return APISuccess(saved_booking)
}

// func (t *MealRequestServlet) notify_guest(booking *PopupBooking) (error) {
// 	guest, err := GetGuestById(t.db, booking.Guest_id)
// 	if err != nil {
// 		log.Println(err)
// 		return err
// 	}
// 	phone, err := GetPhoneForGuest(t.db, guest.Id)
// 	if err != nil {
// 		log.Println(err)
// 	}
// 	if phone != "" {
// 		err := t.text_guest(phone, booking)
// 		if err != nil {
// 			return err
// 		}
// 	} 
// 	err = t.send_guest_confirmation(booking)
// 	return err
// }

// func (t *MealRequestServlet) notify_host(booking *PopupBooking) (error) {
// 	// Let them see 
// 	// 
// 	// 
// }
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
/*
curl --data "method=TestEmailGuest&bookingId=68" https://qa.yaychakula.com/api/mealrequest
*/
func (t *MealRequestServlet) TestEmailGuest(r *http.Request) *ApiResult {
	booking_id_s := r.Form.Get("bookingId")
	booking_id, err := strconv.ParseInt(booking_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed booking ID", 400)
	}
	booking, err := GetBookingById(t.db, booking_id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to retrieve booking", 500)
	}
	if err := t.send_guest_confirmation(booking); err != nil {
		log.Println(err)
		return APIError("Failed to send email", 500)
	}
	return APISuccess("OK")
}

func (t *MealRequestServlet) send_guest_confirmation(booking *PopupBooking) error {
	popup, err := GetPopupById(t.db, booking.Popup_id)
	if err != nil {
		return err
	}
	meal, err := GetMealById(t.db, popup.Meal_id)
	if err != nil {
		return err
	}
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		return err
	}
	guest, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		return err
	}
	guest_email, err := GetEmailForGuest(t.db, guest.Id)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("%s Welcomed You to %s!", 
			host_as_guest.First_name, 
			meal.Title)
	html_buf, err := ioutil.ReadFile(server_config.HTML.Path + "meal_confirmation.html")
	if err != nil {
		return err
	}
	html := string(html_buf)
	message := sendgrid.NewMail()
    message.AddTo(guest_email)
    message.AddToName(guest.First_name)
    message.SetSubject(subject)
    message.SetHTML(html)
    message.SetFrom("meals@yaychakula.com")

    message.AddSubstitution(":time", BuildTime(popup.Starts))
    message.AddSubstitution(":address", popup.Address)
    message.AddSubstitution(":city", popup.City)
    message.AddSubstitution(":state", popup.State)
    message.AddSubstitution(":meal_id", fmt.Sprintf("%d", meal.Id))
    message.AddSubstitution(":price", 
    	fmt.Sprintf("%.2f", booking.Meal_price * float64(booking.Seats)))
    return t.sg_client.Send(message)
}

func (t *MealRequestServlet) TestNotifyHostBooking(r *http.Request) *ApiResult {
	booking_id_s := r.Form.Get("bookingId")
	booking_id, err := strconv.ParseInt(booking_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed booking ID", 400)
	}
	booking, err := GetBookingById(t.db, booking_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid booking ID", 400)
	}
	if err := t.notify_host_booking(booking); err != nil {
		log.Println(err)
		return APIError("Coud not notify host", 400)
	}
	return APISuccess("OK")
}

func (t *MealRequestServlet) notify_host_booking(booking *PopupBooking) error {
	// get the host as guest
	// get the host-guest's email
	// get the guest information
	// get the html template
	// fill in the substitutions
	meal, err := GetMealByPopupId(t.db, booking.Popup_id)
	if err != nil {
		return err
	}
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		return err
	}
	host_email, err := GetEmailForGuest(t.db, host_as_guest.Id)
	if err != nil {
		return err
	}
	attendee, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("New Booking for %s!", meal.Title)
	html_buf, err := ioutil.ReadFile(server_config.HTML.Path + "notify_host_booking.html")
	if err != nil {
		return err
	}
	html := string(html_buf)
	if server_config.Version.V != "prod" {
		html = "<p><strong>This is a test message that does not apply to any activity on your Chakula account</strong></p>" +
				html
		subject = "[TEST] " + subject
	}
	message := sendgrid.NewMail()
    message.AddTo(host_email)
    message.AddToName(host_as_guest.First_name)
    message.SetSubject(subject)
    message.SetHTML(html)
    message.SetFrom("meals@yaychakula.com")
    message.AddSubstitution(":meal_name", meal.Title)
    message.AddSubstitution(":guest_name", attendee.First_name)
    message.AddSubstitution(":seat_count", fmt.Sprintf("%d", booking.Seats))
    guestlist_html, err := t.generate_guestlist_html_for_popup(booking.Popup_id)
    if err != nil {
    	return err
    }
    message.AddSubstitution("{guest_list}", guestlist_html)
    return t.sg_client.Send(message)
}

func (t *MealRequestServlet) generate_guestlist_html_for_popup(popup_id int64) (string, error) {
	// get guests for popup
	attendees, err := GetAttendeesForPopup(t.db, popup_id)
	if err != nil {
		return "", err
	}
	guestlist_html := ""
	for _, attendee := range attendees {
		guestlist_html += 
			fmt.Sprintf("<p><span style='font-family:lucida sans unicode,lucida grande,sans-serif;'>" + 
				"%s, %d seats</p>",
				attendee.Guest.First_name, attendee.Seats)
	}
	return guestlist_html, nil
}
func (t *MealRequestServlet) process_popup_charge_worker() {
	// get all meals that happened 7 - 8 days ago
	if server_config.Version.V != "prod" { // only run this routine on prod
		log.Println("Exiting meal_charge routine on qa")
		return
	}	
	for {
		t.process_meal_charges()
		time.Sleep(time.Hour)
	}
}
// // curl --data "method=ProcessMealCharges"
// func (t *MealRequestServlet) ProcessMealCharges(r *http.Request) *ApiResult {
// 	t.process_meal_charges()
// 	return APISuccess("Ok")
// }
// TODO: handle failed charges...

func (t *MealRequestServlet) process_meal_charges(){
	window_starts := time.Now().Add(-time.Hour * 72)
	window_ends := time.Now().Add(-time.Hour * 48)
	popups, err := GetPopupsFromTimeWindow(t.db, window_starts, window_ends)
	if err != nil {
		log.Println(err)
		return
	}
	for _, popup := range popups {
		t.process_popup(popup)
	}
}
/*
curl --data "method=ProcessPopup&popupId=16" https://yaychakula.com/api/mealrequest
*/
func (t *MealRequestServlet) ProcessPopup(r *http.Request) *ApiResult {
	popup_id_s := r.Form.Get("popupId")
	popup_id, err := strconv.ParseInt(popup_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed popup ID", 400)
	}
	popup, err := GetPopupById(t.db, popup_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid popup ID", 400)
	}
	if popup.Processed == 1 {
		return APISuccess("Already processed")
	}
	if err = t.process_popup(popup); err != nil {
		log.Println(err)
		return APIError("Failed to process popup", 400)
	}
	return APISuccess("OK")
}

func (t *MealRequestServlet) process_popup(popup *Popup) error {
	if (popup.Processed == 1) { // skip the processed meals
		return nil
	}
	bookings, err := GetBookingsForPopup(t.db, popup.Id)
	if err != nil {
		return err
	}
	if err = t.process_bookings(bookings); err != nil {
		return err
	}
	SetPopupProcessed(t.db, popup.Id)
	return t.notify_host_payment_processed(popup) // TO QA
}
/*
curl --data "method=TestNotifyHostPayment&popupId=1" https://yaychakula.com/api/mealrequest
*/

func (t *MealRequestServlet) TestNotifyHostPayment(r *http.Request) *ApiResult {
	popup_id_s := r.Form.Get("popupId")
	popup_id, err := strconv.ParseInt(popup_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed popup ID", 400)
	}
	popup, err := GetPopupById(t.db, popup_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid popup ID", 400)
	}
	if err := t.notify_host_payment_processed(popup); err != nil {
		log.Println(err)
		return APIError("Coud not notify host", 400)
	}
	return APISuccess("OK")
}

// WORKING
func (t *MealRequestServlet) notify_host_payment_processed(popup *Popup) error {	
	meal, err := GetMealById(t.db, popup.Meal_id)
	if err != nil {
		return err
	}
	bookings, err := GetBookingsForPopup(t.db, popup.Id)
	if err != nil {
		return err
	}
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		return err
	}
	host_as_guest.Email, err = GetEmailForGuest(t.db, host_as_guest.Id)
	if err != nil {
		return err
	}
	subject := "Processed: " + meal.Title
	html_buf, err := ioutil.ReadFile(server_config.HTML.Path + "notify_host_popup_processed.html")
	if err != nil {
		return err
	}
	html := string(html_buf)
	if server_config.Version.V != "prod" {
		subject = "[TESTING]" + subject
		html = "<p><strong>THIS IS A TEST. " + 
				"This does reflect actual activity related to your Chakula account.</strong></p>" +
				html
	}
	message := sendgrid.NewMail()
    message.AddTo(host_as_guest.Email)
    message.AddToName(host_as_guest.First_name)
    message.SetSubject(subject)
    message.SetHTML(html)
    message.SetFrom("meals@yaychakula.com")
    message.AddSubstitution(":time", BuildTime(popup.Starts))
    invoice_html := t.get_guest_list_receipt_html(bookings, meal)
    message.AddSubstitution("{invoice_list}", invoice_html)
    if err := t.sg_client.Send(message); err != nil {
        return err
    }
    return nil
}

// TO QA
func (t *MealRequestServlet) get_guest_list_receipt_html(bookings []*PopupBooking, meal *Meal) string {
	html := ""
	total := float64(0)
	for _, booking := range bookings {
		guest, err := GetGuestById(t.db, booking.Guest_id)
		if err != nil {
			log.Println(err)
			return ""
		}
		html += 
			fmt.Sprintf("<p> %s: %d seats</p>", guest.First_name, booking.Seats)
		total += meal.Price * float64(booking.Seats)
	}
	html += fmt.Sprintf("<p> Total: $%.2f</p>", total)
	return html
}

func (t *MealRequestServlet) process_bookings(bookings []*PopupBooking) error {
	for _, booking := range bookings {
		err := t.charge_booking(booking)
		if err != nil {
			return err
		}
	}
	return nil
}

/*
curl https://api.stripe.com/v1/charges \
   -u sk_live_6DdxsleLP40YnpsFATA1ZJCg: \
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
curl --data "method=ChargeBooking&id=78&key=askjasdasdfasdf43fDFGj4tF2345d34nfnrj3333lsdfjLLKJd" https://yaychakula.com/api/mealrequest
curl https://connect.stripe.com/oauth/token \
   -d client_secret=sk_test_PsKvXuwitPqYwpR7hPse4PFk \
   -d refresh_token=REFRESH_TOKEN \
   -d grant_type=refresh_token
*/
func (t *MealRequestServlet) ChargeBooking(r *http.Request) *ApiResult {
	booking_id_s := r.Form.Get("id")
	booking_id, err := strconv.ParseInt(booking_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Processing error", 400)
	}

	key := r.Form.Get("key")
	if key != "askjasdasdfasdf43fDFGj4tF2345d34nfnrj3333lsdfjLLKJd" {
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
   -u sk_live_6DdxsleLP40YnpsFATA1ZJCg: \
   -d amount=___ \
   -d currency=usd \
   -d customer=___ \
   -d destination=___ \
   -d application_fee=___
*/
// qa'd
func (t *MealRequestServlet) charge_booking(booking *PopupBooking) error {
	// Get customer object, meal (to get the price), and host (to get stripe destination)
	if booking.Last4 == 0 { // skip dummy and complementary bookings
		return nil
	}
	customer, err := GetStripeTokenByGuestIdAndLast4(t.db, booking.Guest_id, booking.Last4)
	if err != nil {
		log.Println(err)
		return err
	}
	guest, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		log.Println(err)
		return err
	}
	meal, err := GetMealByPopupId(t.db, booking.Popup_id)
	if err != nil {
		log.Println(err)
		return err
	}
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return err
	}
	host_price_pennies := meal.Price * 100
	seats := float64(booking.Seats)
	total_pennies := int(booking.Meal_price * seats * 100)
	chakula_fee_pennies := total_pennies - int(host_price_pennies * seats)
	log.Println("Price in pennies: ", host_price_pennies)
	log.Println("Total in pennies: ", total_pennies)
	log.Println("Chakula fee in pennies: ", chakula_fee_pennies)
	description := guest.First_name + "'s payment for " + meal.Title
	return PostStripeCharge(total_pennies, 
		chakula_fee_pennies, 
		customer.Stripe_token, 
		host.Stripe_user_id,
		description)
}

func PostStripeCharge(total, chakula_fee int, customer_token, host_account, description string) error {
	client := &http.Client{}
   	stripe_body := url.Values{
		"amount": {strconv.Itoa(total)},
		"currency": {"usd"},
		"customer": {customer_token},
		"destination": {host_account},
		"application_fee": {strconv.Itoa(chakula_fee)},
		"description": {description},
	}
	req, err := http.NewRequest(
		"POST",
		"https://api.stripe.com/v1/charges",
		strings.NewReader(stripe_body.Encode()))
	if err != nil {
		log.Println(err)
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if server_config.Version.V == "prod" {
		req.SetBasicAuth("sk_live_6DdxsleLP40YnpsFATA1ZJCg:", "")
	} else {
		req.SetBasicAuth("sk_test_PsKvXuwitPqYwpR7hPse4PFk:", "")
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return err
	}
	if resp.StatusCode != 200 {
		return errors.New(resp.Status)
	}
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