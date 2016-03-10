package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"strconv"
	"time"
	"math/rand"
	"math"
	"strings"
	"encoding/json"
	"os"
	"fmt"
	"io/ioutil"
	"errors"
)

type MealServlet struct {
	db              *sql.DB
	server_config   *Config
	session_manager *SessionManager
	random          *rand.Rand
}

type Meal_read struct {
	Id 				int64
	Title 			string
	Description		string
	Host_name 		string
	Host_pic		string
	Host_bio		string
	Address 		string
	City 			string
	State 			string
	Open_spots 		int64
	Price			float64
	Status 			string
	Maps_url 		string
	Follows_host 	bool
	Host_id 		int64
	Cards 			[]int64
	New_host 		bool
	Published 		bool
	Attendees 		[]*Attendee_read
	Starts			time.Time
	Rsvp_by			time.Time
	Pics 			[]*Pic
	Meal_reviews	[]*Review
	Host_reviews 	[]*Review_read
	Popups 			[]*Popup
	Upcoming_meals 	[]*Meal_read	
}

type Popup struct {
	Id 			int64
	Starts 		time.Time
	Rsvp_by 	time.Time
	Address 	string
	City 		string
	State 		string
	Meal_id 	int64
	Capacity 	int64
	Processed 	int64
	// Go Fields
	Attendees 	[]*Attendee_read
	Maps_url	string
	Attending 	bool
}

type Review_read struct {
	First_name 		string
	Prof_pic_url 	string
	Meal_id 		int64
	Meal_title 		string
	Rating 			int64
	Comment 		string
	Date 			time.Time
}

func NewMealServlet(server_config *Config, session_manager *SessionManager) *MealServlet {
	t := new(MealServlet)
	t.server_config = server_config
	db, err := sql.Open("mysql", server_config.GetSqlURI())
	if err != nil {
		log.Fatal("NewMealServlet", "Failed to open database:", err)
	}
	t.random = rand.New(rand.NewSource(time.Now().UnixNano()))

	t.db = db
	t.session_manager = session_manager
	return t
}

/*
curl --data "method=getUpcomingMeals" https://yaychakula.com/api/meal
*/
// type Home_Meals struct {
// 	Upcoming_meals 	[]*Meal_read
// 	Attending_meals []*Meal_read
// }

// func (t *MealServlet) GetUpcomingMeals(r *http.Request) *ApiResult {
// 	home_meals := new(Home_Meals)
// 	home_meals.Upcoming_meals, err := GetUpcomingMealsFromDB(t.db)
// 	if err != nil {
// 		log.Println(err)
// 		return APIError("Failed to retrieve meals", 500)
// 	}
// 	session_id := r.Form.Get("session")
// 	if session == "" {
// 		return APISuccess(upcoming_meals)
// 	}
// 	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
// 	if err != nil {
// 		log.Println(err)
// 		return APIError("Failed to retrieve meals", 500)
// 	}
// 	if !session_valid {
// 		log.Println(session_valid)
// 		return nil, err
// 	}
// 	home_meals.Attending_meals := GetUpcomingAttendingMealsForGuest(t.db, session.Guest.Id)
// 	// get session
// 	// if there is one get attending meals for that guest 
// 	// append them to upcoming_meals, OR create a custom "home page struct" to store attending meals and upcoming meals
// 	return APISuccess(upcoming_meals)

// 	// get all the meals where RSVP time > now
// 	// return the array
// }

func (t *MealServlet) GetUpcomingMeals(r *http.Request) *ApiResult {
	// home_meals := new(Home_Meals)
	upcoming_meals, err := GetUpcomingMealsFromDB(t.db)
	if err != nil {
		log.Println(err)
		return APIError("Failed to retrieve meals", 500)
	}
	// session_id := r.Form.Get("session")
	// if session == "" {
	// 	return APISuccess(upcoming_meals)
	// }
	// session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	// if err != nil {
	// 	log.Println(err)
	// 	return APIError("Failed to retrieve meals", 500)
	// }
	// if !session_valid {
	// 	log.Println(session_valid)
	// 	return nil, err
	// }
	// home_meals.Attending_meals := GetUpcomingAttendingMealsForGuest(t.db, session.Guest.Id)
	// get session
	// if there is one get attending meals for that guest 
	// append them to upcoming_meals, OR create a custom "home page struct" to store attending meals and upcoming meals
	return APISuccess(upcoming_meals)

	// get all the meals where RSVP time > now
	// return the array
}


func GetMealPriceById(db *sql.DB, meal_id int64) (float64, error) {
	meal, err := GetMealById(db, meal_id)
	if err != nil {
		return 0, err
	}
	return GetMealPrice(db, meal)
}

func GetMealPrice(db *sql.DB, meal *Meal) (float64, error) {
	new_host, err := GetNewHostStatus(db, meal.Host_id)
	if err != nil {
		return float64(0), err
	}
	if new_host {
		return meal.Price, nil
	}
	return GetMealPriceWithCommission(meal.Price), nil
}

func GetMealPriceWithCommission(price float64) float64 {
	if price <= 15 {
		return price * 1.28
	} else if price < 100 {
		commission_percent := (-0.152941 * price + 30.2941)/100
		return price * (1 + commission_percent)
	}
	return price * 1.15
}

func GetMealIdFromReq(r *http.Request) (int64, error) {
	meal_id_s := r.Form.Get("mealId")
	return strconv.ParseInt(meal_id_s, 10, 64)
}

type Attendee_read struct {
	First_name 		string
	Prof_pic_url	string
	Seats 			int64
	Bio 			string
}

// curl --data "method=GetMealAttendees&mealId=3" https://qa.yaychakula.com/api/meal
func (t *MealServlet) get_popup_attendees(meal_id int64) ([]*Attendee_read, error) {
	attendees, err := GetAttendeesForPopup(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	attendee_reads := make([]*Attendee_read, 0)
	for _, attendee := range attendees {
		as_guest := attendee.Guest
		attendee_read := new(Attendee_read)
    	attendee_read.First_name = as_guest.First_name
    	if as_guest.Prof_pic != "" {
    		attendee_read.Prof_pic_url = "https://yaychakula.com/img/" + as_guest.Prof_pic
    	} else if as_guest.Facebook_id != "" {
    		attendee_read.Prof_pic_url = GetFacebookPic(attendee.Guest.Facebook_id)
    	}
    	attendee_read.Seats = attendee.Seats
    	attendee_read.Bio = as_guest.Bio
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

// SENDGRID API KEY: ***REMOVED***
// SENDGRID PASSWORD: ***REMOVED***
// curl -X POST https://api.sendgrid.com/api/mail.send.json -d api_user=agree -d api_key=***REMOVED*** -d to="agree.ahmed@gmail.com" -d toname=Agree -d subject=Testing -d text="Hey Agree, Can you let me know if this worked?" -d from=agree@yaychakula.com

// curl --data "method=getMealDraft&mealId=3&session=f1caa66a-3351-48db-bcb3-d76bdc644634" https://qa.yaychakula.com/api/meal
func (t* MealServlet) GetMealDraft(r *http.Request) *ApiResult {
	// get session
	session_id := r.Form.Get("session")
	host, err := GetHostBySession(t.db, t.session_manager, session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	// get meal id
	meal_id, err := GetMealIdFromReq(r)	
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal id", 400)
	}
	// make sure the host id matches the meal draft
	meal_draft, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not retrieve draft", 400)
	}
	if meal_draft.Host_id != host.Id {
		return APIError("This is not your meal draft", 400)
	}
	// get pics
	pics, err := GetMealPics(t.db, meal_draft.Id)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal id", 400)
	}
	meal_draft.Pics = pics
	// if meal is published, price and start time fields should be disabled
	return APISuccess(meal_draft)
	// if no, error
	// if yes, say yes
}

// Called by browser to fetch all meals for host.
/* 
curl --data "method=GetMealsForHost&session=f1caa66a-3351-48db-bcb3-d76bdc644634" https://qa.yaychakula.com/api/meal
*/
func (t *MealServlet) GetMealsForHost(r *http.Request) *ApiResult {
	session := r.Form.Get("session")
	host, err := GetHostBySession(t.db, t.session_manager, session)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	meals, err := GetMealsForHost(t.db, host.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate meals", 400)
	}
	for _, meal := range meals {
		meal.Pics, err = GetMealPics(t.db, meal.Id)
		if err != nil {
			log.Println(err)
			continue
		}
	}
	return APISuccess(meals)
}

// Currently can be called on a meal that has already been published
func (t *MealServlet) PublishMeal(r *http.Request) *ApiResult {
	// get meal
	meal_id, err := GetMealIdFromReq(r)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal Id", 400)
	}
	meal_draft, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid meal Id", 400)
	}
	// get host by user's session
	session := r.Form.Get("session")
	host, err := GetHostBySession(t.db, t.session_manager, session)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	// recache homepage
	// check that the host is the author of this meal draft
	if meal_draft.Host_id != host.Id {
		return APIError("You are not the author of this meal", 400)
	}
	_, err = t.db.Exec(`
		UPDATE Meal
		SET Published = 1
		WHERE Id = ?
		`,
		meal_id,
	)
	if err != nil {
		log.Println(err)
		return APIError("Failed to publish meal", 500)
	}
	return APISuccess(meal_id)
}

func (t *MealServlet) DeleteMeal(r *http.Request) *ApiResult {
	meal_id, err := GetMealIdFromReq(r)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal id", 400)
	}
	session_id := r.Form.Get("session")
	host, err := GetHostBySession(t.db, t.session_manager, session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)		
	}
	meal, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate meal", 400)
	}
	if meal.Host_id != host.Id {
		return APIError("This is not your meal", 400)		
	}
	popups, _ := GetPopupsForMeal(t.db, meal_id)
	for _, popup := range popups {
		if meal.Published && popup.Attendees != nil && len(popup.Attendees) > 0 {
			return APIError("You cannot delete a published meal. Please contact agree@yaychakula.com if you need assistance.", 400)
		}
	}
    _, err = t.db.Exec("DELETE FROM Meal WHERE Id = ?", meal_id)
    if err != nil {
    	log.Println(err)
		return APIError("Failed to delete meal. Please contact agree@yaychakula.com", 400)
    }
    return APISuccess("Okay")
}

func (t *MealServlet) CreatePopup(r *http.Request) *ApiResult {
	meal_id_s := r.Form.Get("MealId")
	meal_id, err := strconv.ParseInt(meal_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal id", 400)
	}
	meal, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate meal", 400)
	}
	session_id := r.Form.Get("Session")
	host, err := GetHostBySession(t.db, t.session_manager, session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	if meal.Host_id != host.Id {
		log.Println(err)
		return APIError("You are not the host of this meal", 400)
	}

	popup := new(Popup)
	popup.Meal_id = meal_id
	// parse seats
	seats_s := r.Form.Get("Capacity")
	popup.Capacity, err = strconv.ParseInt(seats_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed seat count", 400)
	}
	// and start time
	starts_s := r.Form.Get("Starts")
	starts_int, err := strconv.ParseInt(starts_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed start time", 400)
	}
	popup.Starts = time.Unix(starts_int, 0)

	// and rsvp by time
	rsvp_by_s := r.Form.Get("Rsvp_by")
	rsvp_by_int, err := strconv.ParseInt(rsvp_by_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed rsvp by time", 400)
	}
	popup.Rsvp_by = time.Unix(rsvp_by_int, 0)
	popup.Address = r.Form.Get("Address")
	popup.City = r.Form.Get("City")
	popup.State = r.Form.Get("State")
	full_address := 
		fmt.Sprintf("%s, %s, %s", 
			popup.Address, 
			popup.City, 
			popup.State)
	err = t.geocode_location(full_address)	
	if err != nil {
		log.Println(err)
		return APIError("Could not confirm your address. Please check it and try again", 400)
	}
	_, err = CreatePopup(t.db, popup)
	if err != nil {
		log.Println(err)
		return APIError("Could not create popup. Please check it and try again", 400)
	}
	popup.Attendees = make([]*Attendee_read, 0)
	return APISuccess(popup)
}
// Maybe add safeguard that prevents hosts from updating starts or price on already published meals
func (t *MealServlet) SaveMealDraft(r *http.Request) *ApiResult {
	// and price
	price_s := r.Form.Get("Price")
	price, err := strconv.ParseFloat(price_s, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed price", 400)
	}
	// get the host data based on the session
	session_id := r.Form.Get("Session")
	host, err := GetHostBySession(t.db, t.session_manager, session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}

	// create the meal draft 
	meal_draft := new(Meal)
	meal_draft.Host_id = host.Id
	meal_draft.Title = r.Form.Get("Title")
	meal_draft.Description = r.Form.Get("Description")
	// meal_draft.Capacity = seats
	meal_draft.Price = price
	// if there's no id, create a new meal
	// if there is an id, update an existing meal
	id_s := r.Form.Get("Meal_id")
	var id int64
	if id_s == "" {
		// create a meal
		id, err = t.create_meal_draft(meal_draft)
		if err != nil {
			return APIError("Failed to create meal", 400)
		} 
	} else { 
		id, err = strconv.ParseInt(id_s, 10, 64)
		if err != nil {
			log.Println(err)
			return APIError("Malformed id", 400)
		}
		meal_draft.Id = id
		// Safeguard against hosts updating price or start time of a published meal
		err = UpdateMeal(t.db, meal_draft)
		// TODO: check if meal is published, recache it
		if err != nil {
			// assume there is no rows, create
			log.Println(err)
			return APIError("Failed to update meal", 500)
		}
	}
	pics := r.Form.Get("Pictures")
	jsonBlob := []byte(pics)
	log.Println(pics)
	err = t.process_pics(jsonBlob, id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to load pictures. Please try again.", 500)
	}
	return APISuccess(id)
}

func (t *MealServlet) create_meal_draft(meal_draft *Meal) (int64, error) {
	result, err := CreateMeal(t.db, meal_draft)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// Takes json blob of pic data and meal id
// checks each pic to see if it is a new upload or a previous one
// if it's a new upload, creates the pic file
func (t *MealServlet) process_pics(json_blob []byte, meal_id int64) error {
	existing_pics := make([]Pic, 0)
	new_pics := make([]Pic, 0)
	pics_to_save := make([]Pic, 0)
	err := json.Unmarshal(json_blob, &pics_to_save)
	if err != nil {
		return err
	}
	for _, pic := range pics_to_save {
		if strings.HasPrefix(pic.Name, "data:image") { 
			new_pics = append(new_pics, pic)
		} else { // pic is already stored on server
			existing_pics = append(existing_pics, pic)
		}
	}
	err = t.update_database_pics(existing_pics, meal_id)
	if err != nil {
		return err
	}
	// pic is a new upload, create a file for it
	log.Println("Creating new image file")
	return t.create_meal_pic_files(new_pics, meal_id)
}

func (t *MealServlet) create_meal_pic_files(pics []Pic, meal_id int64) error {
	for _, pic := range pics {
		err := t.create_meal_pic_file(pic, meal_id)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *MealServlet) create_meal_pic_file(pic Pic, meal_id int64) error {
	file_name, err := CreatePicFile(pic.Name)
	if err != nil {
		return err
	}
	return SaveMealPic(t.db, file_name, pic.Caption, meal_id)
}

// takes id of meal, array of picture file names
// compares the array of stored pics sent by the user with the list of stored pics in the db
// deletes all pictures from the server and db if they were not part of the array of pics sent by the user
func (t *MealServlet) update_database_pics(submitted_pics []Pic, meal_id int64) error {
	database_pics, err := GetMealPics(t.db, meal_id)
	if err != nil {
		return err
	}
	for _, db_pic := range database_pics {
		// if it's not in the existing pics passed by user,
		// delete from the img directory and delete from the MealPic table
		keep_db_pic, err := t.sync_with_submitted_pics(submitted_pics, db_pic)
		if err != nil {
			return err
		}
		if !keep_db_pic {
    		err := os.Remove("/var/www/prod/img/" + db_pic.Name)
      		if err != nil {
    	      return err
      		}
      		_, err = t.db.Exec("DELETE FROM MealPic WHERE Name = ?", db_pic.Name)
      		if err != nil {
    	      	return err
      		}
		}
	}
	return nil
}

// takes an individual pic from the db
// checks if it is among the pics the user submitted
// if it is and has a different caption, updates the caption in the db
// returns true if the db pic should be kept, false if it should be deleted
func (t *MealServlet) sync_with_submitted_pics(existing_pics []Pic, db_pic *Pic) (bool, error) {
	keep_db_pic := false
	for _, existing_pic := range existing_pics {
		if existing_pic.Name == db_pic.Name {
			keep_db_pic = true
			if existing_pic.Caption != db_pic.Caption {
				_, err := t.db.Exec("UPDATE MealPic SET Caption = ? WHERE Name = ?", 
								existing_pic.Caption, 
								existing_pic.Name)
				if err != nil {
					return keep_db_pic, err
				}
			}
		}
	}
    return keep_db_pic, nil
}

type Location struct {
	Full_address 	string
	Lat 			float64
	Lng 			float64
	Polyline	 	string
}

// takes an address in 123 Easy Street, Springfield, MA format
// (TODO) checks for it in the location table...
// if it's not there, geocodes it and stores it in the db
func (t *MealServlet) geocode_location(full_address string) error {
	_, err := GetLocationByAddress(t.db, full_address)
	if err == sql.ErrNoRows {
		err = t.geocode_new_location(full_address)
	}
	return err
}

func (t *MealServlet) Geocode(r *http.Request) *ApiResult {
	lat, err := strconv.ParseFloat(r.Form.Get("lat"), 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed lat", 400)
	}
	lng, err := strconv.ParseFloat(r.Form.Get("lng"), 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed lng", 400)
	}
	return APISuccess(t.generate_polyline_code(lat,lng))
}

// takes an address in 123 Easy Street, Springfield, MA format
// gets the lat + lon from the google maps API 
// generates the polyline code for the circle around a fuzzied nearby center pt
// and stores it in DB
func (t *MealServlet) geocode_new_location(full_address string) error {
	url_address := strings.Replace(full_address, " ", "+", -1)
	resp, err := 
		http.Get("https://maps.googleapis.com/maps/api/geocode/json?" + 
			"address=" + url_address + 
			"&key=AIzaSyDejzDPKNoMHqIuD33_5e53SEXT0zPJ6ww")
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	var f interface{}
	err = json.Unmarshal(b, &f)
	if err != nil {
		return err
	}
	response := f.(map[string]interface{})
	// check status
	if (response["status"] != "OK") {
		return errors.New(response["status"].(string))
	}
	results := response["results"].([]interface{})[0]
	geometry := results.(map[string]interface{})["geometry"]
	location_type := geometry.(map[string]interface{})["location_type"]
	// make sure it's an exact address
	if (location_type != "ROOFTOP") {
		return errors.New("Could not locate address")
	}
	// get and store location
	location := geometry.(map[string]interface{})["location"]
	lat := location.(map[string]interface{})["lat"].(float64)
	lng := location.(map[string]interface{})["lng"].(float64)
	polyline := t.get_polycode_for_location(lat, lng)
	return StoreLocation(t.db, lat, lng, full_address, polyline)
}

// takes lat and lon
// fuzzes a new center < 500m from location
// generates the 360 pts of a circle around fuzzed center
// encodes it using the polyline encoding algorthim
// returns polyline encoded string
func (t *MealServlet) get_polycode_for_location(lat, lng float64) string {
	fuzz_lat, fuzz_lng := t.get_fuzzied_center(lat, lng)
	return t.generate_polyline_code(fuzz_lat, fuzz_lng)
}

// takes lat and lng
// returns new coordinate < 250m N/S, & <250m E/W of the lat and lng 
// from: http://stackoverflow.com/questions/7477003/calculating-new-longtitude-latitude-from-old-n-meters
func (t *MealServlet) get_fuzzied_center(lat, lng float64) (new_lat, new_lng float64) {
	r_earth := 6378000
	dy := t.random.Int() % 250
	dx := t.random.Int() % 250
	if (rand.Int() % 1 == 0) {
		dy *= -1
	}
	if (rand.Int() % 1 == 0) {
		dx *= -1
	}
	fuzz_lat := float64(dy) / float64(r_earth) * float64(180 / math.Pi)
	fuzz_lng := float64(dx) / float64(r_earth) * float64(180 / math.Pi) / math.Cos(lat * math.Pi/180)
	new_lat = lat + fuzz_lat
	new_lng = lng + fuzz_lng
	return new_lat, new_lng
}

type Point struct {
	Lat 	float64
	Lng 	float64
}

// from: http://stackoverflow.com/questions/7316963/drawing-a-circle-google-static-maps
func (t *MealServlet) generate_polyline_code(lat, lng float64) string {
	r_earth := 6371000
 	pi := math.Pi

 	lat = (lat * pi) / 180
 	lng = (lng * pi) / 180
 	d := float64(250) / float64(r_earth) // diameter of circle

 	points := make([]*Point, 0)
 	// generate the points. Adjust granularity using the incrementor 
	for i := 0; i <= 360; i ++ {
	   brng := float64(i) * pi / float64(180)
	   log.Println(i)
	   p_lat := math.Asin(math.Sin(lat) * math.Cos(d) + math.Cos(lat) * math.Sin(d) * math.Cos(brng))
	   p_lng := ((lng + math.Atan2(math.Sin(brng) * math.Sin(d) * math.Cos(lat), 
	   						math.Cos(d) - math.Sin(lat) * math.Sin(p_lat))) * 180) / pi
	   p_lat = (p_lat * 180) / pi
	   point := new(Point)
	   point.Lat = p_lat
	   point.Lng = p_lng
	   log.Println(p_lat)
	   log.Println(p_lng)
	   points = append(points, point)
	}
	return EncodePolyline(points)
}

func EncodePolyline(coordinates []*Point) string {
	if len(coordinates) == 0 {
		return ""
	}

	factor := math.Pow(10, 5)
	output := encode(coordinates[0].Lat, factor) + encode(coordinates[0].Lng, factor)

	for i := 1; i < len(coordinates); i++ {
		a := coordinates[i]
		b := coordinates[i-1]
		output += encode(a.Lat-b.Lat, factor)
		output += encode(a.Lng-b.Lng, factor)
	}
	log.Println(output)
	return output
}

func encode(oldCoordinate float64, factor float64) string {
	coordinate := int(math.Floor(oldCoordinate*factor + 0.5))
	coordinate = coordinate << 1

	if coordinate < 0 {
		coordinate = ^coordinate
	}
	output := ""
	for coordinate >= 0x20 {
		runeC := string((0x20 | (coordinate & 0x1f)) + 63)
		output = output + runeC
		coordinate >>= 5
	}
	runeC := string(coordinate + 63)
	output = output + runeC
	return output
}


/*
curl --data "method=bookMeal&mealId=4&session=" https://yaychakula.com/api/meal
*/
// func (t *MealServlet) BookMeal(r *http.Request) *ApiResult {
// 	// LITERALLY the same exact call as request meal, with status preset 1, no communication involved.
// }

/*

curl --data "method=getMeal&mealId=60&session=ce5fddc6-7d81-4092-a996-9f157f99fafe" https://yaychakula.com/api/meal
*/

func (t *MealServlet) GetMeal(r *http.Request) *ApiResult{
	
	// parse the meal id
	meal_id, err := GetMealIdFromReq(r)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal ID", 400)
	}
	// check if the meal is published
	meal, err := GetMealById(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Invalid meal ID", 400)
	}
	// get the data from the db and populate the fields required for a listing
	meal_data, err := GetMealCardDataById(t.db, meal.Id)
	meal_data.Published = meal.Published
	meal_data.Popups, err = GetUpcomingPopupsForMeal(t.db, meal.Id)
	if err != nil && err != sql.ErrNoRows {
		log.Println(err)
		return APIError("Failed to load attendees", 500)
	}
	meal_data.Host_reviews = t.get_host_reviews(meal_data.Host_id)
	if err != nil {
		log.Println(err)
	}
	meal_data.Upcoming_meals, err = GetUpcomingMealsFromDB(t.db)
	if err != nil {
		log.Println(err)
	}
	// get the guest's session
	session_id := r.Form.Get("session")
	if session_id == "" {
		meal_data.Status = "NONE"
		if !meal.Published {
			log.Println("tried to access unpublished meal without session")
			return APIError("Could not load meal", 400)
		}
	} else {
		meal_data, err = t.getMealWithGuestInfo(meal_data, meal, session_id)
		if err != nil {
			log.Println(err)
			return APIError("Could not load meal", 400)
		}
		log.Println(meal_data.Follows_host)
	}
	log.Println(meal_data.Follows_host)
	return APISuccess(meal_data)
}

func (t *MealServlet) getMealWithGuestInfo(meal_data *Meal_read, meal *Meal, session_id string) (*Meal_read, error) {
	session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		return nil, err
	}
	if !meal.Published {
		host, err := GetHostByGuestId(t.db, session.Guest.Id)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		if host.Id != meal.Host_id {
			return nil, errors.New("This is not your meal")
		}
	}
	meal_data.Follows_host = GetGuestFollowsHost(t.db, session.Guest.Id, meal_data.Host_id)
	log.Println(meal_data.Follows_host)
	meal_data.Cards, err = GetLast4sForGuest(t.db, session.Guest.Id) 
	for i, popup := range meal_data.Popups {
		attendees, err := GetAttendeesForPopup(t.db, popup.Id)
		if err != nil {
			log.Println(err)
			return nil, err
		}

		for _, attendee := range attendees {
			if attendee.Guest.Id == session.Guest.Id {
				// NEED TO FIGURE OUT A WAY TO EXPOSE THE ADDRESS FOR THIS POPUP
				popup.Maps_url = 
					fmt.Sprintf("https://maps.googleapis.com/maps/api/staticmap?" + 
						"size=600x300&scale=2&zoom=14&markers=color:red|%s,%s,%s", 
						popup.Address, popup.City, popup.State)
				popup.Attending = true
				meal_data.Popups[i] = popup
				break
			}
		}
		if !popup.Attending { // show the fuzzy map and hide address
			popup.Maps_url, err = GetStaticMapsUrlForMeal(t.db, popup.Address + ", " + popup.City + ", " + popup.State)
			if err != nil {
				return nil, err
			}
			popup.Address = ""
		}
	}
	return meal_data, nil
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
		meal, err := GetMealByPopupId(t.db, review.Popup_id)
		if err != nil {
			log.Println(err)
			return nil
		}
		review_read.First_name = guest.First_name
		if guest.Prof_pic != "" {
			review_read.Prof_pic_url = "https://yaychakula.com/img/" + guest.Prof_pic 
		} else if guest.Prof_pic == "" && guest.Facebook_id != "" {
			review_read.Prof_pic_url = GetFacebookPic(guest.Facebook_id)
		}
		review_read.Rating = review.Rating
		review_read.Comment = review.Comment
		review_read.Date = review.Date
		review_read.Meal_id = meal.Id
		review_read.Meal_title = meal.Title
		review_reads = append(review_reads, review_read)
	}
	return review_reads
}
