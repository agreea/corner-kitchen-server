package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"fmt"
	"time"
	"net/url"
	"strings"
    "github.com/sendgrid/sendgrid-go"
    "io/ioutil"
)

type ReviewServlet struct {
	db              *sql.DB
	server_config   *Config
	session_manager *SessionManager
	twilio_queue    chan *SMS
	sg_client 		*sendgrid.SGClient // sendGrid client
}


func NewReviewServlet(server_config *Config, session_manager *SessionManager, twilio_queue chan *SMS) *ReviewServlet {
	t := new(ReviewServlet)
	t.server_config = server_config
	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewMealRequestServlet", "Failed to open database:", err)
	}
	t.db = db
	t.session_manager = session_manager
	t.twilio_queue = twilio_queue
	t.sg_client = sendgrid.NewSendGridClient(server_config.SendGrid.User, server_config.SendGrid.Pass)

	// go t.nudge_review_worker()
	return t
}

func (t *ReviewServlet) nudge_review_worker(){
	for {
		if server_config.Version.V != "prod" {
			log.Println("Continuing as qa. Not filling out email")
			time.Sleep(time.Hour * 100)
			break
		}
		log.Println("In the nudge review loop")
		t.nudge_review_for_recent_meals()
		time.Sleep(time.Hour)
	}
}

// TODO FIX THIS JESUS - POPUP INTEGRATION
func (t *ReviewServlet) nudge_review_for_recent_meals(){
	window_starts := time.Now().Add(-time.Hour * 24 * 6) // starts should be farther back in the past than the ends
	window_ends := time.Now().Add(-time.Hour * 2)
	popups, err := GetPopupsFromTimeWindow(t.db, window_starts, window_ends)
	if err != nil {
		log.Println(err)
		return
	}
	for _, popup := range popups {
		bookings, err := GetBookingsForPopup(t.db, popup.Id)
		if err != nil {
			log.Println(err)
			return
		}
		if err = t.nudge_attendees(bookings, popup); err != nil {
			log.Println(err)
		}
	}
}

// Called for each meal
func (t *ReviewServlet) nudge_attendees(bookings []*PopupBooking, popup *Popup) error {
// for each booking:
// 	if the nudge count < 3 && time since last nudge > 2 days
// 		email guest nudge
// 		set last nudge to now and increase nudge count by 1

	meal, err := GetMealByPopupId(t.db, bookings[0].Popup_id)
	if err != nil {
		return err
	}
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		return err
	}
	for _, booking := range bookings {
		if booking.Nudge_count == -1 {
			continue
		}
		if (time.Since(booking.Last_nudge) > time.Hour * 48 && booking.Nudge_count < 3) {
			if err = t.nudge_attendee(meal, booking, host_as_guest); err != nil {
				return err
			}
		}
	}
	return nil
}
func (t *ReviewServlet) nudge_attendee(meal *Meal, booking *PopupBooking, host_as_guest *GuestData) error {
	attendee, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		return err	
	}
	attendee_email, err := GetEmailForGuest(t.db, booking.Guest_id)
	if err != nil {
		return err	
	}
	subject := fmt.Sprintf("%s, Please Review %s", attendee.First_name, meal.Title)
	if booking.Nudge_count > 1 {
		subject = "Reminder: " + subject
	}
	html_buf, err := ioutil.ReadFile("html/review_nudge.html")
	if err != nil {
		return err
	}
	html := string(html_buf)
	message := sendgrid.NewMail()
	message.AddTo(attendee_email)
	message.AddToName(attendee.First_name)
	message.SetSubject(subject)
	message.SetHTML(html)
	message.SetFrom("meals@yaychakula.com")
	message.AddSubstitution(":attendee_name", attendee.First_name)
	message.AddSubstitution(":meal_title", meal.Title)
	message.AddSubstitution(":popup_id", fmt.Sprintf("%d", booking.Popup_id))
	message.AddSubstitution(":host_name", host_as_guest.First_name)
	if err = t.sg_client.Send(message); err != nil {
		return err
	}
	_, err = t.db.Exec(`UPDATE PopupBooking
		SET Nudge_count = Nudge_count + 1 AND Last_nudge = ?
		WHERE Id = ?
		`,
		time.Now(),
		booking.Id,)
	return err
}
/*
SENDGRID API KEY: ***REMOVED***
SENDGRID PASSWORD: ***REMOVED***
"<p> Hi Agree </p><p> Can you tell me if this worked? </p>"
"<p>Hi %s,</p><p>Thank you for attending %s's meal. We hope you enjoyed it.</p><p>We hope that you will take the time to <a href=https://yaychakula.com/review.html?Id=%d>review your meal here</a>. It will help %s build their reputation and will strengthen our little Chakula community.</p><p>We are so happy that you are part of the Chakula movement.</p><p>Have a good one!</p><p> Agree and Pat </p>"
curl -X POST https://api.sendgrid.com/api/mail.send.json -d api_user=agree -d api_key=***REMOVED*** -d to="agree.ahmed@gmail.com" -d toname=Agree -d subject=Testing -d html="<p>Hi %s,</p><p>Thank you for attending %s's meal. We hope you enjoyed it.</p><p>We hope that you will take the time to <a href=https://yaychakula.com/review.html?Id=%d>review your meal here</a>. It will help %s build their reputation and will strengthen our little Chakula community.</p><p>We are so happy that you are part of the Chakula movement.</p><p>Have a good one</p><p> Agree and Pat </p>" -d from=agree@yaychakula.com
*/

func SendEmail(email_address string, subject string, html string) {
	client := &http.Client{}
   	sendgrid_body := url.Values{
		"api_user": {"agree"},
		"api_key": {"***REMOVED***"},
		"to": {email_address},
		"toname":{"Chakula"},
		"subject": {subject},
		"html": {html},
		"from": {"meals@yaychakula.com"},
	}
	req, err := http.NewRequest(
		"POST",
		"https://api.sendgrid.com/api/mail.send.json",
		strings.NewReader(sendgrid_body.Encode()))
	if err != nil {
		log.Println(err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(resp)
}

func (t *ReviewServlet) notify_attendee_to_review(guest_id int64, meal_id int64) {
	meal, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return
	}
	guest, err := GetGuestById(t.db, guest_id)
	if err != nil {
		log.Println(err)
		return
	}
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return		
	}

	if guest.Phone != "" {
		msg := new(SMS)
		msg.To = guest.Phone
		// Heyo! Make sure to you review %s's %s so they can build their reputation. Here's the link:
		msg.Message = fmt.Sprintf("Hey! Make sure you review %s's %s so they can build their reputation. Here's the link: https://yaychakula.com/review.html?Id=%d",
			host_as_guest.First_name, meal.Title,
			meal.Id)
		t.twilio_queue <- msg
	} else {

	}
	// get the guest
	// 
}

func (t *ReviewServlet) GetReviewData(r *http.Request) *ApiResult {
	session_id := r.Form.Get("session")
	session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate guest", 400)
	}
	popup_id_s := r.Form.Get("popupId")
	popup_id, err := strconv.ParseInt(popup_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed popup ID", 400)
	}
	// if popup.Starts > moment() return error
	// check that the guest had requested the meal
	booking, err := GetBookingByGuestAndPopupId(t.db, session.Guest.Id, popup_id)
	if err != nil {
		log.Println(err)
		return APIError("You can only review meals you have attended", 400)
	}
	meal, err := GetMealByPopupId(t.db, popup_id)
	if err != nil {
		log.Println(err)
		return APIError("You can only review meals you have attended", 400)		
	}
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 500)		
	}
	meal_read := new(Meal_read)
	meal_read.Title = meal.Title
	meal_read.Host_name = host_as_guest.First_name
	meal_read.Price = booking.Meal_price
	return APISuccess(meal_read)
}
// curl -d 'to=destination@example.com&amp;toname=Destination&amp;subject=Example Subject&amp;text=testingtextbody&amp;from=info@domain.com&amp;api_user=agree&amp;api_key=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' https://api.sendgrid.com/api/mail.send.json
// Secret key: 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

// worker goes like this:

// get all the meals that started between 2.5 and 3.5 hours ago
// get all the guests for all those meals
// contact all the guests with review links for that meal
// contact goes like this:
// if you have their phone number, text them
// else send them an email
// sleep for 1 hour.
// curl --data "method=ChargeTip&reviewId=34" https://yaychakula.com/api/review
func (t *ReviewServlet) ChargeTip(r *http.Request) *ApiResult {
	review_id_s := r.Form.Get("reviewId")
	review_id, err := strconv.ParseInt(review_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed review ID", 400)
	}
	review, err := GetReviewById(t.db, review_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate review", 400)
	}
	booking, err := GetBookingByGuestAndPopupId(t.db, review.Guest_id, review.Popup_id)
	err = t.charge_tip(review.Tip_percent, booking)
	if err != nil {
		log.Println(err)
		return APIError("Failed to charge tip", 500)
	}
	return APISuccess("OK")
}
// TODO: getReviewForm()
// TODO: check that star rating is set on front end
// takes meal id returns host, meal title....
// curl --data "method=leaveReview&comment=blah&suggestion=blahblabla&rating=5&tipPercent=20&session=1234&popupId=15" https://qa.yaychakula.com/api/review
func (t *ReviewServlet) PostReview(r *http.Request) *ApiResult {
	// get the session's guest
	session_id := r.Form.Get("session")
	session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate guest", 400)
	}
	review := new(Review)
	review.Guest_id = session.Guest.Id
	popup_id_s := r.Form.Get("popupId")
	review.Popup_id, err = strconv.ParseInt(popup_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed popup ID", 400)
	}
	// TODO: if popup.Starts > moment() return error
	// check that the guest had requested the meal
	booking, err := GetBookingByGuestAndPopupId(t.db, session.Guest.Id, review.Popup_id)
	if err != nil {
		log.Println(err)
		return APIError("You can only review meals you have attended", 400)
	}
	// check the table: have they left a review already?
	saved_review, err := GetMealReviewByGuestIdAndPopupId(t.db, session.Guest.Id, review.Popup_id)
	if saved_review != nil {
		return APIError("You only review a meal once. That's the motto, #YORAMO", 400)
	}
	rating_s := r.Form.Get("Rating")
	review.Rating, err = strconv.ParseInt(rating_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed rating", 400)
	}
	tip_percent_s := r.Form.Get("TipPercent")
	review.Tip_percent, err = strconv.ParseInt(tip_percent_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed tip percent", 400)
	}
	// StripeCharge tip %
	if review.Tip_percent != 0 {
		err = t.charge_tip(review.Tip_percent, booking)
		if err != nil {
			log.Println(err)
			return APIError("Failed to charge tip.", 500)
		}
	}
	review.Comment = r.Form.Get("Comment")
	review.Suggestion = r.Form.Get("Suggestion")
	err = SaveReview(t.db, review)
	if err != nil {
		log.Println(err)
		return APIError("Failed to save review. Please try again.", 500)
	}
	t.notify_host(review)
	return APISuccess("OK")
}

func (t *ReviewServlet) charge_tip(tip_percent int64, booking *PopupBooking) error {
	tip_percent_float := float64(tip_percent) / float64(100)
	total_tip := tip_percent_float * booking.Meal_price * float64(booking.Seats)
	meal, err := GetMealByPopupId(t.db, booking.Popup_id)
	if err != nil {
		return err
	}
	guest, err := GetGuestById(t.db, booking.Guest_id)
	if err != nil {
		return err
	}
	host_tip := meal.Price * tip_percent_float
	chakula_fee := total_tip - host_tip + 0.3
	customer, err := GetStripeTokenByGuestIdAndLast4(t.db, booking.Guest_id, booking.Last4)
	if err != nil {
		return err
	}
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		return err
	}
	description := guest.First_name + "'s gratuity for " + meal.Title

	PostStripeCharge(int(total_tip * 100), 
		int(chakula_fee * 100), 
		customer.Stripe_token, 
		host.Stripe_user_id,
		description)
	return nil
}
// /*
// curl --data "method=TestNotifyHost&reviewId=0" https://qa.yaychakula.com
// */
// func (t *ReviewServlet) TestNotifyHost(r *http.Request) *ApiResult {
// 	review_id_s := r.Form.Get("reviewId")
// 	review_id, err := strconv.ParseInt(review_id_s, 10, 64)
// 	if err != nil {
// 		log.Println(err)
// 		return APIError("Malformed review ID", 400)
// 	}
// 	review, err := GetReviewById(t.db, review_id)
// 	if err != nil {
// 		log.Println(err)
// 		return APIError("Malformed review ID", 400)
// 	}
//     if err := t.notify_host(review); err != nil {
// 		log.Println(err)
//         return APIError("Could not notify host", 500)
//     }
//     return APISuccess("OK")
// }

func (t *ReviewServlet) notify_host(review *Review) error {
	meal, err := GetMealByPopupId(t.db, review.Popup_id)
	if err != nil {
		return err
	}
	message, err := t.build_review_notif_email(meal, review)
	if err != nil {
		return err
	}
    message = t.append_tip_section(message, review, meal)
    message = t.append_suggestion_section(message, review)
    if err := t.sg_client.Send(message); err != nil {
        return err
    }
    return nil
}

func (t *ReviewServlet) build_review_notif_email(meal *Meal, review *Review) (*sendgrid.SGMail, error) {
	host_as_guest, err := GetGuestByHostId(t.db, meal.Host_id)
	if err != nil {
		return nil, err
	}	
	guest, err := GetGuestById(t.db, review.Guest_id)
	if err != nil {
		return nil, err
	}
	host_email, err := GetEmailForGuest(t.db, host_as_guest.Id)
	if err != nil {
		return nil, err
	}
	html_buf, err := ioutil.ReadFile("html/review_email.html")
	if err != nil {
		return nil, err
	}
	subject := "You Have a New Review from " + guest.First_name
	html := string(html_buf)
	if server_config.Version.V != "prod" {
		subject = "[TESTING] " + subject
		html = "<p><strong>This is a test message and does not reflect" +
			" activity on your Chakula account</strong></p>" + html
	}
	message := sendgrid.NewMail()
    message.AddTo(host_email)
    message.AddToName(host_as_guest.First_name)
    message.SetSubject(subject)
    message.SetHTML(html)
    message.SetFrom("reviews@yaychakula.com")
    message.AddSubstitution(":guest", guest.First_name)
    message.AddSubstitution(":comment", review.Comment)
    message.AddSubstitution(":host_id", fmt.Sprintf("%d", meal.Host_id))
    // message.AddSubstitution(":review_id", 0) // TODO: add review id to review passed in
    return message, nil

}
func (t *ReviewServlet) append_tip_section(message *sendgrid.SGMail, review *Review, meal *Meal) *sendgrid.SGMail {
    if (review.Tip_percent > 0) {
		tip_amount := float64(review.Tip_percent)/float64(100) * meal.Price
		gratuity_section := 
			fmt.Sprintf("<h4>Gratuity<h4><p>$%.2f (%d percent)</p>",
				tip_amount, 
				review.Tip_percent)
    	message.AddSubstitution("{gratuity}", gratuity_section)
    } else {
    	message.AddSubstitution("{gratuity}", "")
    }
    return message
}

func (t *ReviewServlet) append_suggestion_section(message *sendgrid.SGMail, review *Review) *sendgrid.SGMail {
    if (review.Suggestion != "") {
    	message.AddSubstitution("{suggestion}", 
    		"<h4>Suggestion<h4>" +
    		"<p>This will only be visible to you</p>" +
    		"<p>:suggestion<p>")
    	message.AddSubstitution(":suggestion", review.Suggestion)
    } else {
    	message.AddSubstitution("{suggestion}", "")
    }
    return message
}

func (t *ReviewServlet) Get(r *http.Request) *ApiResult {
	id_s := r.Form.Get("id")
	id, err := strconv.ParseInt(id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed id", 400)
	}
	review, err := GetReviewById(t.db, id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to get review", 400)
	}
	review_read := new(Review_read)
	guest, err := GetGuestById(t.db, review.Guest_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	meal, err := GetMealByPopupId(t.db, review.Popup_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	review_read.First_name = guest.First_name
	review_read.Prof_pic_url = GetFacebookPic(guest.Facebook_id)
	review_read.Rating = review.Rating
	review_read.Comment = review.Comment
	review_read.Date = review.Date
	review_read.Meal_id = meal.Id
	review_read.Meal_title = meal.Title
	return APISuccess(review_read)
}
/*
Get averag

*/

/*
curl --data "method=getReviewsForMeal&mealId=3" https://qa.yaychakula.com/api/review
*/

/*
curl --data "method=getReviewsForHost&hostId=1" https://qa.yaychakula.com/api/review
*/

/*
curl --data "method=getReviewsForGuest&guestId=1" https://qa.yaychakula.com/api/review
*/

/*
curl --data "method=getMeal&session=f1caa66a-3351-48db-bcb3-d76bdc644634&mealId=3" https://qa.yaychakula.com/api/meal
*/