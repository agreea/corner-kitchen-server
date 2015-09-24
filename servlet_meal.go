package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"time"
)

type MealServlet struct {
	db              *sql.DB
	server_config   *Config
	session_manager *SessionManager
}

type Attendee struct {
	First_name 		string
	Prof_pic_url	string
}

type MealData struct {
	Title 			string
	Description		string
	Host_name 		string
	Host_pic		string
	Open_spots 		int64
	Price			float64
	Status 			string
	Attendees 		[]*Attendee
	Starts			time.Time
	Rsvp_by			time.Time
	Pics 			[]*Pic		
}

func NewMealServlet(server_config *Config, session_manager *SessionManager) *MealServlet {
	t := new(MealServlet)
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
func (t *MealServlet) GetUpcomingMeals(r *http.Request) *ApiResult {
	meals, err := GetUpcomingMealsFromDB(t.db)
	if err != nil {
		log.Println(err)
		return APIError("Failed to retrieve meals", 500)
	}
	return APISuccess(meals)
	// get all the meals where RSVP time > now
	// return the array
}

// curl --data "method=GetMealAttendees&mealId=3" https://qa.yaychakula.com/api/meal
func (t *MealServlet) get_meal_attendees(meal_id int64) ([]*Attendee, error) {
	guests, err := GetAttendeesForMeal(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	attendees := make([]*Attendee, 0)
	for guest := range guests {
    	attendee := new(Attendee)
    	attendee.First_name = guests[guest].First_name
    	attendee.Prof_pic_url = GetFacebookPic(guests[guest].Facebook_id)
    	attendees = append(attendees, attendee)
	}
	return attendees, nil
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
func (t *MealServlet) GetMeal(r *http.Request) *ApiResult{
	// parse the meal id
	meal_id_s := r.Form.Get("mealId")
	meal_id, err := strconv.ParseInt(meal_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	// get the meal
	meal, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid meal ID", 400)
	}
	// use host to get guest
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return APIError("There was an error. Please try again", 500)
	}
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return APIError("Server error", 500)
	}
	meal_data := new(MealData)
	meal_data.Title = meal.Title
	meal_data.Description = meal.Description
	meal_data.Price = meal.Price
	meal_data.Host_name = host_as_guest.First_name
	meal_data.Host_pic = GetFacebookPic(host_as_guest.Facebook_id)
	meal_data.Starts = meal.Starts
	meal_data.Rsvp_by = meal.Rsvp_by
	guest_ids, err := GetAttendeesForMeal(t.db, meal.Id)
	if err != nil {
		log.Println(err)
		return APIError("Server error", 500)
	}
	pics, err := GetPicsForMeal(t.db, meal.Id)
	if err != nil {
		log.Println(err)
	}
	attendees, err := t.get_meal_attendees(meal.Id)
	if err == nil {
		meal_data.Attendees = attendees
	}
	meal_data.Pics = pics
	meal_data.Open_spots = meal.Capacity - int64(len(guest_ids))
	// get the guest's session
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		meal_data.Status = "NONE"
		log.Println(err)
		return APIError("Could not process session", 500)
	}
	if !session_valid {
		meal_data.Status = "NONE"
		log.Println(session_valid)
		return APISuccess(meal_data)
	}
	// get the request, if there is one. Show this in the status
	meal_req, err := t.get_request_by_guest_and_meal_id(session.Guest.Id, meal_id)
	if err != nil {
		log.Println(err)
		meal_data.Status = "NONE"
	} else if meal_req.Status == 0 {
		meal_data.Status = "PENDING"
	} else if meal_req.Status == 1 {
		meal_data.Status = "ATTENDING"
	} else if meal_req.Status == -1 {
		meal_data.Status = "DECLINED"
	}
	return APISuccess(meal_data)
}

func (t *MealServlet) get_request_by_guest_and_meal_id(guest_id int64, meal_id int64) (meal_request *MealRequest, err error) {
	meal_req, err := GetMealRequestByGuestIdAndMealId(t.db, guest_id, meal_id)
	if err != nil {
		return nil, err
	}
	return meal_req, nil
}


