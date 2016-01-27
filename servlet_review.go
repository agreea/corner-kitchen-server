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
)

type ReviewServlet struct {
	db              *sql.DB
	server_config   *Config
	session_manager *SessionManager
	twilio_queue    chan *SMS
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
	// go t.nudge_review_worker()
	return t
}

func (t *ReviewServlet) nudge_review_worker(){
	for {
		if server_config.Version.V == "qa" {
			log.Println("Continuing as qa. Not filling out email")
			time.Sleep(time.Hour * 100)
			break
		}
		log.Println("In the nudge review loop")
		t.nudge_review_for_recent_meals()
		time.Sleep(time.Hour)
	}
}

func (t *ReviewServlet) nudge_review_for_recent_meals(){
	window_starts := time.Now().Add(-time.Hour * 24 * 6) // starts should be farther back in the past than the ends
	window_ends := time.Now().Add(-time.Hour * 2)
	meals, err := GetMealsFromTimeWindow(t.db, window_starts, window_ends)
	if err != nil {
		log.Println(err)
		return
	}
	for _, meal := range meals {
		requests, err := GetConfirmedMealRequestsForMeal(t.db, meal.Id)
		if err != nil {
			log.Println(err)
			return
		}
		err = t.nudge_attendees(requests)
		if err != nil {
			log.Println(err)
		}
	}
}

// Called for each meal
func (t *ReviewServlet) nudge_attendees(requests []*MealRequest) error {
	meal, err := GetMealById(t.db, requests[0].Meal_id)
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
	for _, request := range requests {
		if request.Nudge_count == -1 {
			continue
		}
		attendee, err := GetGuestById(t.db, request.Guest_id)
		if err != nil {
			log.Println(err)
			continue
		}
		// nudge the attendee if they haven't reviewed the meal in 48 hours 
		if (time.Since(request.Last_nudge) > time.Hour * 48 && request.Nudge_count < 3) {
			if (request.Nudge_count == 0 && attendee.Phone != "") { // text if we have their phone and it's the first nudge
				msg := new(SMS)
				msg.To = attendee.Phone
				msg.Message = fmt.Sprintf("Heyo! Thanks for coming to %s's meal! Make sure you leave a review so %s can build their reputation." +
											" Here's the link: https://yaychakula.com/review.html?Id=%d %0a" +
											"Love, Chakula",
					host_as_guest.First_name, host_as_guest.First_name,
					meal.Id)
				t.twilio_queue <- msg
			} else { // email them
				attendee.Email, err = GetEmailForGuest(t.db, attendee.Id)
				subject := fmt.Sprintf("%s, Please Review %s's Meal", attendee.First_name, host_as_guest.First_name)
				if request.Nudge_count > 1 {
					subject = "Reminder: " + subject
				}
				html := fmt.Sprintf("<p>Hi %s,</p>" +
									"<p>Thank you for attending %s's meal.</p>" + 
									"<p>We hope that you will take the time to <a href=https://yaychakula.com/review.html?Id=%d>review your meal here</a>. " +
									"Your review will help %s build a reputation and will strengthen our little Chakula community.</p>" +
									"<p>We are so happy that you are part of the Chakula movement.</p>" +
									"<p>Have a good one!</p>" +
									"<p> Agree and Pat </p>",
									attendee.First_name, host_as_guest.First_name, meal.Id, host_as_guest.First_name)
				SendEmail(attendee.Email, subject, html)
			}
			_, err = t.db.Exec(`
				UPDATE MealRequest
				SET Nudge_count = Nudge_count + 1 AND Last_nudge = ?
				WHERE Id = ?
				`,
				time.Now(),
				request.Id,
			)
			if err != nil {
				log.Println(err)
				return err
			}
		}
	}
	return nil
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

// TODO: getReviewForm()
// TODO: check that star rating is set on front end
// TODO: build a form for new meals.
// takes meal id returns host, meal title....
// curl --data "method=getUpcomingMeals" https://qa.yaychakula.com/api/meal
func (t *ReviewServlet) LeaveReview(r *http.Request) *ApiResult {
	// get the session's guest
	session_id := r.Form.Get("session")
	valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Couldn't locate guest", 400)
	}
	if !valid {
		return APIError("Invalid session", 400)
	}
	meal_id_s := r.Form.Get("mealId")
	meal_id, err := strconv.ParseInt(meal_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	// check that the guest had requested the meal
	request, err := GetMealRequestByGuestIdAndMealId(t.db, session.Guest.Id, meal_id)
	if err != nil || request.Status != 1 {
		log.Println(err)
		return APIError("You can only review meals you have attended", 400)
	}
	// check the table: have they left a review already?
	review, err := GetMealReviewByGuestIdAndMealId(t.db, session.Guest.Id, meal_id)
	if review != nil {
		return APIError("You only review a meal once. That's the motto, #YORAMO", 400)
	}
	rating_s := r.Form.Get("rating")
	rating, err := strconv.ParseInt(rating_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed rating", 400)
	}
	tip_percent_s := r.Form.Get("tipPercent")
	tip_percent, err := strconv.ParseInt(tip_percent_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed tip percent", 400)
	}
	comment := r.Form.Get("comment")
	err = SaveReview(t.db, session.Guest.Id, meal_id, rating, comment, tip_percent)
	if err != nil {
		return APIError("Failed to save review. Please try again.", 500)
	}
	return APISuccess("OK")
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
	meal, err := GetMealById(t.db, review.Meal_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	review_read.First_name = guest.First_name
	review_read.Prof_pic_url = GetFacebookPic(guest.Facebook_id)
	review_read.Rating = review.Rating
	review_read.Comment = review.Comment
	review_read.Date = review.Date
	review_read.Meal_id = review.Meal_id
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