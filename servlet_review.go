package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"fmt"
	// "time"
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
	return t
}

// func (t *ReviewServlet) review_notifier() {
// 	// get every meal that's happened recently
// 	for { 
// 		meals, err := GetMealsToNotifyForReview(t.db)
// 		if err != nil {
// 			log.Println(err)
// 		}
// 		// get every attendee for every meal
// 		for _, meal := range meals { 
// 			attendees, err := GetAttendeesForMeal(t.db, meal.Id)
// 			if err!= nil {
// 				log.Println(err)
// 				time.Sleep(1 * time.Hour)
// 				continue
// 			}
// 			for _, attendee := range attendees { // remind every attendee to leave a review
// 				t.notify_attendee_to_review(attendee.Id, meal.Id)
// 			}
// 		}
// 	}
// }

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
		msg.Message = fmt.Sprintf("Heyo! Make sure you review %s's %s so they can build their reputation. Here's the link: https://yaychakula.com/review.html?Id=%d",
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

/*
Get averag

*/

/*
curl --data "method=getReviewsForMeal&mealId=3" https://qa.yaychakula.com/api/review
*/

// query the meals table to get all meals hosted by host
// query the review table to get all reviews where meal_id = 1 OR 2 OR ...
// get all the reviews into an array
// iterate over all the ratings: 
// 		get the sum, convert every Guest_id into a First Name and Prof pic
//		
// divide  the sume by the size of the array
// round so that   5 == > 4.7, 4.5 == 4.2 -> 4.7, 4.0 == 3.7 --> 4.2, etc. etc.
// return rating plus an array of review objects

/*
curl --data "method=getReviewsForHost&hostId=1" https://qa.yaychakula.com/api/review
*/

/*
curl --data "method=getReviewsForGuest&guestId=1" https://qa.yaychakula.com/api/review
*/

/*
curl --data "method=getMeal&session=f1caa66a-3351-48db-bcb3-d76bdc644634&mealId=3" https://qa.yaychakula.com/api/meal
*/