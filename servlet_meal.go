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

type Attendee_read struct {
	First_name 		string
	Prof_pic_url	string
	Seats 			int64
}

type MealData struct {
	Id 				int64
	Title 			string
	Description		string
	Host_name 		string
	Host_pic		string
	Host_bio		string
	Open_spots 		int64
	Price			float64
	Status 			string
	Attendees 		[]*Attendee_read
	Starts			time.Time
	Rsvp_by			time.Time
	Pics 			[]*Pic
	Meal_reviews	[]*Review
	Host_reviews 	[]*Review_read		
}

type Review_read struct {
	First_name 		string
	Prof_pic_url 	string
	Meal_id 		int64
	Rating 			int64
	Comment 		string
	Date 			time.Time
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


/*

type Meal struct {
	Id      		int64
	Host_id 		int64
	Price   		float64
	Title   		string
	Description		string
	Capacity		int64
	Starts			time.Time
	Rsvp_by			time.Time
}

type MealData struct {
	Title 			string
	Description		string
	Host_name 		string
	Host_pic		string
	Host_bio		string
	Open_spots 		int64
	Price			float64
	Status 			string
	Attendees 		[]*Attendee
	Starts			time.Time
	Rsvp_by			time.Time
	Pics 			[]*Pic
	Meal_reviews	[]*Review
	Host_reviews 	[]*Review_read		
}
*/
// curl --data "method=getUpcomingMeals" https://qa.yaychakula.com/api/meal
func (t *MealServlet) GetUpcomingMeals(r *http.Request) *ApiResult {
	meals, err := GetUpcomingMealsFromDB(t.db)
	if err != nil {
		log.Println(err)
		return APIError("Failed to retrieve meals", 500)
	}
	meal_datas := make([]*MealData, 0)
	for _, meal := range meals {
		meal_data := new(MealData)
		meal_data.Id = meal.Id
		meal_data.Title = meal.Title
		meal_data.Description = meal.Description
		meal_data.Price = meal.Price
		meal_data.Open_spots = meal.Capacity
		meal_data.Starts = meal.Starts
		meal_data.Rsvp_by = meal.Rsvp_by
		meal_data.Pics, err = GetPicsForMeal(t.db, meal.Id)
		if err != nil{ 
			log.Println(err)
		}
		meal_datas = append(meal_datas, meal_data)
	}
	return APISuccess(meal_datas)
	// get all the meals where RSVP time > now
	// return the array
}

// curl --data "method=GetMealAttendees&mealId=3" https://qa.yaychakula.com/api/meal
func (t *MealServlet) get_meal_attendees(meal_id int64) ([]*Attendee_read, error) {
	attendees, err := GetAttendeesForMeal(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	attendee_reads := make([]*Attendee_read, 0)
	for _, attendee := range attendees {
    	attendee_read := new(Attendee_read)
    	attendee_read.First_name = attendee.Guest.First_name
    	attendee_read.Prof_pic_url = GetFacebookPic(attendee.Guest.Facebook_id)
    	attendee_read.Seats = attendee.Seats
    	attendee_reads = append(attendee_reads, attendee_read)
	}
	return attendee_reads, nil
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
curl --data "method=getMeal&session=f1caa66a-3351-48db-bcb3-d76bdc644634&mealId=4" https://qa.yaychakula.com/api/meal
*/

/*
TITLE
ASKING_PRICE
DESCRIPTION
IMAGES...???????????????????????????????????
STARTS_TIME
RSVP_BY_TIME
SESSION
ID (how generated? maybe rand(ms as seed))

create if not there
update if there
*/

// func (t *MealServlet) SaveMealDraft(r *http.Request) *ApiResult{
// 	// 
// }

// get meal draft
// session, ID
// if there is a meal draft with that host id, send it to them
// else error


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
	meal_data.Id = meal.Id
	meal_data.Title = meal.Title
	meal_data.Description = meal.Description
	meal_data.Price = meal.Price
	meal_data.Host_name = host_as_guest.First_name
	meal_data.Host_pic = GetFacebookPic(host_as_guest.Facebook_id)
	meal_data.Host_bio = host.Bio
	meal_data.Starts = meal.Starts
	meal_data.Rsvp_by = meal.Rsvp_by
	meal_data.Host_reviews = t.get_host_reviews(host.Id)
	if err != nil {
		log.Println(err)
	}
	pics, err := GetPicsForMeal(t.db, meal.Id)
	if err != nil {
		log.Println(err)
	}
	attendees, err := t.get_meal_attendees(meal.Id)
	if err == nil {
		taken_seats := int64(0)
		for _, attendee := range attendees {
			taken_seats += attendee.Seats
		}
		meal_data.Attendees = attendees
		meal_data.Open_spots = meal.Capacity - taken_seats
	} else {
		meal_data.Open_spots = meal.Capacity
	}
	meal_data.Pics = pics
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

/*
type Review_read struct {
	First_name 		string
	Prof_pic_url 	string
	Meal_id 		int64
	Rating 			int64
	Comment 		string
	Date 			time.Time
}
*/

func (t *MealServlet) get_host_reviews(host_id int64) ([]*Review_read) {
	host_reviews, err := GetReviewsForHost(t.db, host_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	review_reads := make([]*Review_read, 0)

	for _, review := range host_reviews {
		review_read := new(Review_read)
		guest, err := GetGuestById(t.db, review.Guest_id)
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
		review_reads = append(review_reads, review_read)
	}
	return review_reads
}

func (t *MealServlet) get_request_by_guest_and_meal_id(guest_id int64, meal_id int64) (meal_request *MealRequest, err error) {
	meal_req, err := GetMealRequestByGuestIdAndMealId(t.db, guest_id, meal_id)
	if err != nil {
		return nil, err
	}
	return meal_req, nil
}


