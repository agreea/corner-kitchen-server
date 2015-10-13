package main

import (
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"code.google.com/p/go-uuid/uuid"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"strings"
	"io/ioutil"
	"encoding/base64"
	"encoding/json"
	"os"
	"syscall"
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

type Meal_read struct {
	Id 				int64
	Title 			string
	Description		string
	Host_name 		string
	Host_pic		string
	Host_bio		string
	Open_spots 		int64
	Price			float64
	Status 			string
	Has_email		bool
	Attendees 		[]*Attendee_read
	Starts			time.Time
	Rsvp_by			time.Time
	Pics 			[]*Pic
	Meal_reviews	[]*Review
	Host_reviews 	[]*Review_read		
}

type Meal_draft struct {
	Id 				int64
	Title 			string
	Description 	string
	Price 			string
	Seats 			string
	Pics 			[]*Pic
	Starts 			time.Time
	Rsvp_by 		time.Time
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
	go t.process_meal_charge_worker()
	return t
}

/*
curl --data "method=getUpcomingMeals" https://qa.yaychakula.com/api/meal
*/
func (t *MealServlet) GetUpcomingMeals(r *http.Request) *ApiResult {
	meals, err := GetUpcomingMealsFromDB(t.db)
	if err != nil {
		log.Println(err)
		return APIError("Failed to retrieve meals", 500)
	}
	meal_datas := make([]*Meal_read, 0)
	for _, meal := range meals {
		meal_data := new(Meal_read)
		meal_data.Id = meal.Id
		meal_data.Title = meal.Title
		meal_data.Description = meal.Description
		meal_data.Price = meal.Price * 1.28
		meal_data.Open_spots = meal.Capacity
		meal_data.Starts = meal.Starts
		meal_data.Rsvp_by = meal.Rsvp_by
		meal_data.Pics, err = GetAllPicsForMeal(t.db, meal.Id)
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
// curl -d "method=saveMealDraft&pics=<serialized pic strings>&title=<some title>" https://qa.yaychakula.com/api/meal
/*
type Meal_draft struct {
	Id 				int64
	Title 			string
	Description 	string
	Price 			string
	Seats 			string
	Pics 			[]*Pic
	Starts 			time.Time
	Rsvp_by 		time.Time
}
*/

func (t* MealServlet) GetMealDraft(r *http.Request) *ApiResult {
	// get session
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	host, err := GetHostByGuestId(t.db, session.Guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	// get meal id
	meal_id_s := r.Form.Get("id")
	meal_id, err := strconv.ParseInt(meal_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal id", 400)
	}
	// make sure the host id matches the meal draft
	meal_draft, err := GetMealDraft(t.db, meal_id)
	if err != nil {
		log.Println(err)
		return APIError("Malformed meal id", 400)
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
	return APISuccess(meal_draft)
	// if no, error
	// if yes, say yes
}

func (t *MealServlet) SaveMealDraft(r *http.Request) *ApiResult {
	title := r.Form.Get("title")
	description := r.Form.Get("description")

	// parse seats
	seats_s := r.Form.Get("seats")
	seats, err := strconv.ParseInt(seats_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed seat count", 400)
	}

	// and price
	price_s := r.Form.Get("price")
	price, err := strconv.ParseFloat(price_s, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed price", 400)
	}

	// and start time
	starts_s := r.Form.Get("starts")
	starts_int, err := strconv.ParseInt(starts_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed start time", 400)
	}
	starts := time.Unix(starts_int, 0)

	// and rsvp by time
	rsvp_by_s := r.Form.Get("rsvpBy")
	rsvp_by_int, err := strconv.ParseInt(rsvp_by_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Malformed rsvp by time", 400)
	}
	rsvp_by := time.Unix(rsvp_by_int, 0)

	// get the host data based on the session
	session_id := r.Form.Get("session")
	session_valid, session, err := t.session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}
	if !session_valid {
		return APIError("Invalid session", 400)
	}
	host_as_guest := session.Guest
	host, err := GetHostByGuestId(t.db, host_as_guest.Id)
	if err != nil {
		log.Println(err)
		return APIError("Could not locate host", 400)
	}

	// create the meal draft 
	meal_draft := new(Meal)
	meal_draft.Host_id = host.Id
	meal_draft.Title = title
	meal_draft.Description = description
	meal_draft.Capacity = seats
	meal_draft.Price = price
	meal_draft.Starts = starts
	meal_draft.Rsvp_by = rsvp_by
	// if there's no id, create a new meal
	// if there is an id, update an existing meal
	id_s := r.Form.Get("id")
	var id int64
	if id_s == "" { // there's no ufckin meal
		// create a meal
		result, err := CreateMeal(t.db, meal_draft)
		if err != nil {
			log.Println(err)
			return APIError("Failed to create meal draft", 500)
		}
		id, err = result.LastInsertId()
		if err != nil {
			log.Println(err)
			return APIError("Please try to save your meal again!", 500)
		}
	} else { // there's really an ufckin meal
		id, err = strconv.ParseInt(id_s, 10, 64)
		if err != nil {
			log.Println(err)
			return APIError("Malformed id", 400)
		}
		meal_draft.Id = id
		err = UpdateMeal(t.db, meal_draft)
		if err != nil {
			// assume there is no rows, create
			log.Println(err)
			return APIError("Failed to update meal", 500)
		}
	}
	pics := r.Form.Get("pics")
	jsonBlob := []byte(pics)
	err := t.process_pics(jsonBlob, id)
	if err != nil {
		log.Println(err)
		return APIError("Failed to load pictures. Please try again.", 500)
	}
	// TODO: Load meal into api call... OR just return the id
	return APISuccess("OK")
}

// Takes json blob of pic data and meal id
// checks each pic to see if it is a new upload or a previous one
// if it's a new upload, creates the pic file
func (t *MealServlet) process_pics(json_blob []byte, meal_id int64) error {
	existing_pics := make([]Pic, 0)
	pics_to_save := make([]Pic, 0)
	err := json.Unmarshal(json_blob, &pics_to_save)
	if err != nil {
		return err
	}
	for _, pic := range pics_to_save {
		if strings.HasPrefix(pic.Name, "data:image") { 
		// pic is a new upload, create a file for it
			err := t.create_pic_file(pic.Name)
			if err != nil {
				return err
			}
		} else { // pic is already stored on server
			existing_pics = append(existing_pics, pic)
		}
	}
	if len(existing_pics) > 0 { 
		// update the pics in the db to make sure unwanted ones are deleted 
		// and also that captions are up-to-date
		return t.update_database_pics(existing_pics, meal_id)
	} else {
		return nil	
	}
}

func (t *MealServlet) create_pic_file(pic_string string) error {
	pic_s_split := strings.Split(string(pic_string), "base64,")
	data, err := base64.StdEncoding.DecodeString(pic_s_split[1])
	if err != nil {
		return err
	}
	// extract the file ending from the json encoded string data
	file_ending := strings.Split(pic_s_split[0], "image/")[1]
	file_ending = strings.Replace(file_ending, ";", "", 1) // drop the "images/"
			// generate the file name and address
	file_name := uuid.New() + "." + file_ending
	file_address := "/var/www/prod/img/" + file_name
	log.Println(file_name)
	syscall.Umask(022)
	err = ioutil.WriteFile(file_address, data, os.FileMode(int(0664)))
	if err != nil {
		return err
	}
	// add pic to DB
	return SavePic(t.db, file_name, pic.Caption, meal_id)
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
		keep_db_pic, err := sync_with_submitted_pics(submitted_pics, db_pic)
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
func sync_with_submitted_pics(existing_pics []Pic, db_pic *Pic) (bool, error) {
	existing_pics_contains_db_pic := false
	for _, existing_pic := range existing_pics {
		if existing_pic.Name == database_pic.Name {
			existing_pics_contains = true
			if existing_pic.Caption != database_pic.Caption {
				_, err = t.db.Exec("UPDATE MealPic SET Caption = ? WHERE Name = ?", 
								existing_pic.Caption, 
								existing_pic.Name)
				if err != nil {
					return existing_pics_contains_db_pic, err
				}
			}
		}
	}
    return existing_pics_contains_db_pic, nil
}

func (t *MealServlet) process_meal_charge_worker() {
	// get all meals that happened 7 - 8 days ago
	for {
		t.process_meal_charges()
		time.Sleep(time.Hour * 24)
	}
}

func (t *MealServlet) process_meal_charges(){
	meals, err := GetMealsToProcess(t.db)
	if err != nil {
		log.Println(err)
		return
	}
	for _, meal := range meals {
		meal_reqs, err := GetConfirmedMealRequestsForMeal(t.db, meal.Id)
		if err != nil {
			log.Println(err)
		}
		t.process_meal_requests(meal_reqs)
	}
}

func (t *MealServlet) process_meal_requests(meal_reqs []*MealRequest) {
	for _, meal_req := range meal_reqs {
		// create stripe charge
		t.stripe_charge(meal_req)
	}
	SetMealProcessed(t.db, meal_reqs[0].Meal_id)
}
/*
curl https://api.stripe.com/v1/charges \
   -u ***REMOVED***: \
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

// curl --data "method=issueStripeCharge&id=55" https://qa.yaychakula.com/api/meal
func (t *MealServlet) IssueStripeCharge(r *http.Request) *ApiResult {
	meal_req_id_s := r.Form.Get("id")
	meal_req_id, err := strconv.ParseInt(meal_req_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		return APIError("Ya fucked up", 400)
	}

	meal_req, err := GetMealRequestById(t.db, meal_req_id)
	if err != nil {
		log.Println(err)
		return APIError("Fuck", 500)
	}
	t.stripe_charge(meal_req)
	return APISuccess("OKEN")
}

/*
curl https://api.stripe.com/v1/charges \
   -u ***REMOVED***: \
   -d amount=___ \
   -d currency=usd \
   -d customer=___ \
   -d destination=___ \
   -d application_fee=___
*/

func (t *MealServlet) stripe_charge(meal_req *MealRequest) {
	// Get customer object, meal (to get the price), and host (to get stripe destination)
	customer, err := GetStripeTokenByGuestIdAndLast4(t.db, meal_req.Guest_id, meal_req.Last4)
	if err != nil {
		log.Println(err)
		return
	}

	meal, err := GetMealById(t.db, meal_req.Meal_id)
	if err != nil {
		log.Println(err)
		return	
	}
	log.Println(meal)
	log.Println(meal_req)
	log.Println("Price in pennies: %d", int(meal.Price * 128))
	log.Println("Price in pennies time seats: %d", int(meal.Price * 128) * int(meal_req.Seats))
	host, err := GetHostById(t.db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return
	}
	client := &http.Client{}
   	stripe_body := url.Values{
		"amount": {strconv.Itoa(int(meal.Price * 128) * int(meal_req.Seats))},
		"currency": {"usd"},
		"customer": {customer.Stripe_token},
		"destination": {host.Stripe_user_id},
		"application_fee": {strconv.Itoa(int(meal.Price * 28) * int(meal_req.Seats))},
	}
	req, err := http.NewRequest(
		"POST",
		"https://api.stripe.com/v1/charges",
		strings.NewReader(stripe_body.Encode()))
	if err != nil {
		log.Println(err)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("***REMOVED***:", "")
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println(resp)
	// TODO: react according to Stripe response!
}

/*
curl --data "method=getMeal&session=f1caa66a-3351-48db-bcb3-d76bdc644634&mealId=4" https://qa.yaychakula.com/api/meal
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
	meal_data := new(Meal_read)
	meal_data.Id = meal.Id
	meal_data.Title = meal.Title
	meal_data.Description = meal.Description
	meal_data.Price = meal.Price * 1.28
	meal_data.Host_name = host_as_guest.First_name
	meal_data.Host_pic = GetFacebookPic(host_as_guest.Facebook_id)
	meal_data.Host_bio = host.Bio
	meal_data.Starts = meal.Starts
	meal_data.Rsvp_by = meal.Rsvp_by
	meal_data.Host_reviews = t.get_host_reviews(host.Id)
	if err != nil {
		log.Println(err)
	}
	pics, err := GetAllPicsForMeal(t.db, meal.Id)
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
	meal_data.Has_email = !(session.Guest.Email == "")
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


