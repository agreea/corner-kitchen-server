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
type MealData struct {
	Title 			string
	Description		string
	// Time 			time.Time (?)
	// Rsvp_by			time.Time (?)
	Host_name 		string
	Host_pic		string
	Open_spots 		int64
	Price			float64
	Status 			string
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

// get meal
// get the meal id (done)
// get the session -> guest -> guest id (done)
// get the data from the meal table--title, host, ()
// get the host data--name + profile picture
// get the total spots available - total guests attending ()
// get the meal status for this guest ()
// get the pictures for this meal ()
//
/*
curl --data "method=getMeal&session=f1caa66a-3351-48db-bcb3-d76bdc644634&mealId=1" https://yaychakula.com/api/meal
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


