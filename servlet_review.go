package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"time"
)

type ReviewServlet struct {
	db              *sql.DB
	server_config   *Config
	session_manager *SessionManager
}


func NewReviewServlet(server_config *Config, session_manager *SessionManager) *ReviewServlet {
	t := new(ReviewServlet)
	t.server_config = server_config
	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewMealRequestServlet", "Failed to open database:", err)
	}
	t.db = db
	t.session_manager = session_manager
	return t
}

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
	comment := r.Form.Get("comment")
	err = SaveReview(t.db, session.Guest.Id, meal_id, rating, comment)
	if err != nil {
		return APIError("Failed to save review. Please try again.", 500)
	}
	return APISuccess("OK")
}

/* 
	Review object:
	First_name
	Profile_pic
	Rating
	Comment
	Date
	Meal name
	Meal id
*/

// get guest from session
// write it in the db
// voila
/*
curl --data "method=leaveReview&session=f1caa66a-3351-48db-bcb3-d76bdc644634&mealId=3&rating=5
				&comment=Food was delicious. I absolutely love Izzie's Cuban food." https://qa.yaychakula.com/api/review
*/

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