package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
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
	Pics 			[]string		
}

func NewMealServlet(server_config *Config, session_manager *SessionManager) *MealRequestServlet {
	t := new(MealRequestServlet)
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
		return APIError("Malformed meal ID", 400)
	}
	// use host to get guest
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	host_as_guest, err := GetGuestById(t.db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	meal_data := new(MealData)
	meal_data.Title = meal.Title
	meal_data.Price = meal.Price
	meal_data.Host_name = host_as_guest.Name
	meal_data.Host_pic = GetFacebookPic(host_as_guest.Facebook_id)
	// TODO: calculate open spots
	// get the guest's session
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not process session", 400)
	}
	if !session_valid {
		log.Println(session_valid)
		return APIError("Could not process session", 400)
	}
	// get the request, if there is one
	meal_req, err := t.get_request_by_guest_and_meal_id(meal_id, session.Guest.Id)
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


