package main

import (
	"database/sql"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
	"time"
	"log"
	"errors"
)

type SMS struct {
	To      string
	Message string
}

/*
 * Trucks and meuns
 */

type Truck struct {
	// Raw fields
	Id           int64
	Owner        int64
	Name         string
	Location_lat string
	Location_lon string
	Open_from    time.Time
	Open_until   time.Time
	Phone        string
	Description  string

	// Go fields
	Distance float64
	Menus    []*Menu
	Open_now bool
}

type Menu struct {
	// Raw fields
	Id          int64
	Truck_id    int64
	Name        string
	Description string
	Flagship    bool

	// Go fields
	Items []*MenuItem
}

type MenuItem struct {
	// Raw fields
	Id          int64
	Truck_id    int64
	Menu_id     int64
	Name        string
	Price       float64
	Description string
	Pic_url     string

	ListOptions   []*MenuItemOption
	ToggleOptions []*MenuToggleOption
}

type MenuToggleOption struct {
	Id             int64
	Item_id        int64
	Name           string
	Price_modifier float64
}

type MenuItemOption struct {
	Id      int64
	Item_id int64
	Name    string

	Values []*MenuItemOptionItem
}

type MenuItemOptionItem struct {
	Id             int64
	Option_id      int64
	Option_name    string
	Price_modifier float64
}

type Order struct {
	// Raw fields
	Id          int64
	User_id     int64
	Truck_id    int64
	Date        time.Time
	Pickup_time time.Time
	Phone       string

	// Go fields
	Items []*OrderItem
}

type OrderItem struct {
	// Raw fields
	Id       int64
	Order_id int64
	Item_id  int64
	Quantity int64
	// ID numbers into the toggle / list option tables
	ToggleOptions    []int64
	ListOptionValues []int64
}

// For trucks
type PaymentToken struct {
	Id         int64
	User_id    int64
	Name       string
	stripe_key string
	Token      string
	Created    time.Time
}

type UserData struct {
	// Raw fields
	Id                 int64
	Email              string
	First_name         string
	Last_name          string
	password_hash      string
	password_salt      string
	password_reset_key string
	Phone              string
	Verified           bool

	// Go fields
	orders        []*Order
	Session_token string
}

type Session struct {
	User    *UserData
	Expires time.Time
}

// The Dinner Structs

type GuestData struct {
	Id             		int64
	Email          		string
	First_name          string
	Last_name			string
	Facebook_id    		string
	Facebook_long_token	string
	Phone 				string
	// Go fields
	Session_token 		string
}

type FacebookResp struct {
	Id    		string
	Email 		string
	First_name  string
	Last_name	string
}

type KitchenSession struct {
	Guest   *GuestData
	Expires time.Time
}

type Meal struct {
	Id      		int64
	Host_id 		int64
	Price   		float64
	Title   		string
	Description		string
	Capacity		int64
	Starts			time.Time
	Rsvp_by			time.Time
	Processed 		int64
	Published 		int64
	Pics 			[]*Pic
}

type StripeToken struct {
	Id        		int64
	Guest_id  		int64
	Stripe_token 	string
	Last4			int64
}

type HostData struct {
	Id             			int64
	Guest_id       			int64
	Address        			string
	Stripe_user_id 			string
	Stripe_access_token		string
	Stripe_refresh_token	string
	Bio						string
}

type AttendeeData struct {
	Guest 		*GuestData
	Seats 		int64
}

type MealRequest struct {
	Id       		int64
	Guest_id 		int64
	Meal_id  		int64
	Seats 	 		int64
	Status   		int64
	Last4 	 		int64
	Nudge_count 	int64
	Last_nudge 		time.Time
}

type Review struct {
	Id 			int64
	Guest_id 	int64
	Rating 		int64
	Comment 	string
	Meal_id 	int64
	Date 		time.Time
	Tip_percent int64
}

type Pic struct {
	Name 		string
	Caption 	string
}

func GetUserById(db *sql.DB, id int64) (*UserData, error) {
	row := db.QueryRow(`SELECT Id, Email, First_name, Last_name,
		Password_salt, Password_hash,
		Password_reset_key, Phone, Verified
        FROM User WHERE Id = ?`, id)
	return readUserLine(row)
}

func GetGuestByFbId(db *sql.DB, fb_id string) (*GuestData, error) {
	row := db.QueryRow(`SELECT Id, Email, First_name, Last_name,
		Facebook_id, Facebook_long_token, Phone
		FROM Guest WHERE Facebook_id = ?`, fb_id)
	return readGuestLine(row)
}

func GetGuestById(db *sql.DB, id int64) (*GuestData, error) {
	row := db.QueryRow(`SELECT Id, Email, First_name, Last_name,
		Facebook_id, Facebook_long_token, Phone
		FROM Guest WHERE Id = ?`, id)
	return readGuestLine(row)
}

func GetFacebookPic(fb_id string) string {
	return "https://graph.facebook.com/" + fb_id + "/picture?width=200&height=200"
}

func GetHostByGuestId(db *sql.DB, guest_id int64) (*HostData, error) {
	log.Println(guest_id)
	row := db.QueryRow(`SELECT Id, Guest_id, Address,
		Stripe_user_id, Stripe_access_token, Stripe_refresh_token, Bio 
		FROM Host WHERE Guest_id = ?`, guest_id)
	return readHostLine(row)
}

func GetHostById(db *sql.DB, id int64) (*HostData, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Address, 
		Stripe_user_id, Stripe_access_token, Stripe_refresh_token, Bio 
		FROM Host WHERE Id = ?`, id)
	return readHostLine(row)
}

func GetHostBySession(db *sql.DB, session_manager *SessionManager, session_id string) (*HostData, error) {
	valid, session, err := session_manager.GetGuestSession(session_id)
	if err != nil {
		log.Println(err)
		return nil, errors.New("Couldn't locate guest")
	}
	if !valid {
		return nil, errors.New("Invalid session")
	}
	return GetHostByGuestId(db, session.Guest.Id)
}

func GetUserByEmail(db *sql.DB, email string) (*UserData, error) {
	row := db.QueryRow(`SELECT Id, Email, First_name, Last_name,
		Password_salt, Password_hash,
		Password_reset_key, Phone, Verified
        FROM User WHERE Email = ?`, email)
	return readUserLine(row)
}

func GetUserByPhone(db *sql.DB, phone string) (*UserData, error) {
	row := db.QueryRow(`SELECT Id, Email, First_name, Last_name,
		Password_salt, Password_hash,
		Password_reset_key, Phone, Verified
        FROM User WHERE Phone = ?`, phone)
	return readUserLine(row)
}

func GetMealById(db *sql.DB, id int64) (*Meal, error) {
	row := db.QueryRow(`SELECT Id, Host_id, Price, Title, Description, Capacity, Starts, Rsvp_by, Processed, Published
        FROM Meal 
        WHERE Id = ?`, id)
	return readMealLine(row)
}

func GetMealsFromTimeWindow(db *sql.DB, window_starts time.Time, window_ends time.Time) ([]*Meal, error) {
	rows, err := db.Query(`SELECT Id, Host_id, Price, Title, Description, Capacity, Starts, Rsvp_by, Processed, Published
        FROM Meal 
        WHERE Starts > ? AND Starts < ? AND Published = 1`, 
        window_starts,
        window_ends)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return read_meal_rows(rows)
}

func GetMealsForHost(db *sql.DB, host_id int64) ([]*Meal, error) {
	rows, err := db.Query(`SELECT Id, Host_id, Price, Title, Description, Capacity, Starts, Rsvp_by, Processed, Published
        FROM Meal 
        WHERE Host_id = ?`, 
        host_id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return read_meal_rows(rows)
}

func read_meal_rows(rows *sql.Rows) ([]*Meal, error) {
	meals := make([]*Meal, 0)
	for rows.Next() {
		meal := new(Meal)
		if err := rows.Scan(
			&meal.Id,
			&meal.Host_id,
			&meal.Price,
			&meal.Title,
			&meal.Description,
			&meal.Capacity,
			&meal.Starts,
			&meal.Rsvp_by,
			&meal.Processed,
			&meal.Published,
		); err != nil {
			return nil, err
		}
		meals = append(meals, meal)
	}
	return meals, nil
}

func GetMealRequestByGuestIdAndMealId(db *sql.DB, guest_id int64, meal_id int64) (meal_req *MealRequest, err error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Meal_id, Seats, Status, Last4, Nudge_count, Last_nudge
        FROM MealRequest
        WHERE Guest_id = ? AND Meal_id = ?`, guest_id, meal_id)
	return readMealRequestLine(row)
}

func GetConfirmedMealRequestsForMeal(db *sql.DB, meal_id int64) ([]*MealRequest, error) {
	rows, err := db.Query(`SELECT Id, Guest_id, Meal_id, Seats, Status, Last4, Nudge_count, Last_nudge
        FROM MealRequest
        WHERE Meal_id = ? AND Status = 1`, meal_id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meal_reqs := make([]*MealRequest, 0)
	// get the guest for each guest id and add them to the slice of guests
	for rows.Next() {
		meal_req := new(MealRequest)
		if err := rows.Scan(
			&meal_req.Id,
			&meal_req.Guest_id,
			&meal_req.Meal_id,
			&meal_req.Seats,
			&meal_req.Status,
			&meal_req.Last4,
			&meal_req.Nudge_count,
			&meal_req.Last_nudge,
		); err != nil {
			return nil, err
		}
		meal_reqs = append(meal_reqs, meal_req)
	}
	return meal_reqs, nil
}

// avoids double-charging if both qa and prod are running chronjobs
func SetMealProcessed(db *sql.DB, meal_id int64) error{
	_, err := db.Exec(`
		UPDATE Meal
		SET Processed = 1
		WHERE Id = ?
		`,
		meal_id,
	)
	if err != nil {
		log.Println(err)
	}
	return err
}

func GetMealReviewByGuestIdAndMealId(db *sql.DB, guest_id int64, meal_id int64) (meal_review *Review, err error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Rating, Comment, Meal_id, Date, Tip_percent
        FROM HostReview
        WHERE Guest_id = ? AND Meal_id = ?`, guest_id, meal_id)
	meal_review, err = readMealReviewLine(row)
	if err != nil {
		return nil, err
	}
	return meal_review, nil
}


func GetMealRequestById(db *sql.DB, request_id int64) (*MealRequest, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Meal_id, Seats, Status, Last4, Nudge_count, Last_nudge
        FROM MealRequest
        WHERE Id = ?`, request_id)
	meal_req, err := readMealRequestLine(row)
	if err != nil {
		return nil, err
	}
	return meal_req, nil
}

func GetAttendeesForMeal(db *sql.DB, meal_id int64) ([]*AttendeeData, error) {
	// get all the guest ids attending the meal
	rows, err := db.Query(`
		SELECT Guest_id, Seats
		FROM MealRequest
		WHERE Meal_id = ? AND Status = 1`, meal_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attendees := make([]*AttendeeData, 0)
	// get the guest for each guest id and add them to the slice of guests
	for rows.Next() {
		var guest_id int64
		var seats int64
		if err := rows.Scan(
			&guest_id,
			&seats,
		); err != nil {
			return nil, err
		}
		guest_data, err := GetGuestById(db, guest_id)
		if err == nil {
			attendee := new(AttendeeData)
			attendee.Guest = guest_data
			attendee.Seats = seats
			attendees = append(attendees, attendee)
		} else {
			log.Println(err)
		}
	}
	return attendees, nil
}

func GetUpcomingMealsFromDB(db *sql.DB) ([]*Meal, error) {
	rows, err := db.Query(`
		SELECT Id, Host_id, Price, Title, Description, Capacity, Starts, Rsvp_by
		FROM Meal
		WHERE Rsvp_by > ? AND Id > 0 AND Published = 1`,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meals := make([]*Meal, 0)
	// get the guest for each guest id and add them to the slice of guests
	for rows.Next() {
		meal := new(Meal)
		if err := rows.Scan(
			&meal.Id,
			&meal.Host_id,
			&meal.Price,
			&meal.Title,
			&meal.Description,
			&meal.Capacity,
			&meal.Starts,
			&meal.Rsvp_by,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		meals = append(meals, meal)
	}
	return meals, nil
}

func GetReviewsForHost(db *sql.DB, host_id int64) ([]*Review, error) {
	rows, err := db.Query(`SELECT Id, Host_id, Price, Title, Description, Capacity, Starts, Rsvp_by
        FROM Meal 
        WHERE Host_id = ?`, host_id,
	)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer rows.Close()
	host_reviews := make([]*Review, 0)
	for rows.Next() { // construct each meal, get its reviews, and append them to all host reviews
		meal := new(Meal)
		if err := rows.Scan(
			&meal.Id,
			&meal.Host_id,
			&meal.Price,
			&meal.Title,
			&meal.Description,
			&meal.Capacity,
			&meal.Starts,
			&meal.Rsvp_by,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		meal_reviews, err := GetReviewsForMeal(db, meal.Id)
		if err != nil {
			log.Println(err)
		}
		if meal_reviews != nil {
			host_reviews = append(host_reviews, meal_reviews...)
		}
	}
	return host_reviews, nil
}

func GetReviewsForMeal(db *sql.DB, meal_id int64) ([]*Review, error) {
	rows, err := db.Query(`
		SELECT Id, Guest_id, Rating, Comment, Meal_id, Date, Tip_percent
		FROM HostReview
		WHERE Meal_id = ?`,
		meal_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := make([]*Review, 0)
	// get the guest for each guest id and add them to the slice of guests
	for rows.Next() {
		review := new(Review)
		if err := rows.Scan(
			&review.Id,
			&review.Guest_id,
			&review.Rating,
			&review.Comment,
			&review.Meal_id,
			&review.Date,
			&review.Tip_percent,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func GetReviewByGuestAndMealId(db *sql.DB, guest_id int64, meal_id int64) (*Review, error) {
	row := db.QueryRow(`
		SELECT Id, Guest_id, Rating, Comment, Meal_id, Date, Tip_percent
		FROM HostReview
		WHERE Guest_id = ? AND Meal_id = ?`,
		guest_id,
		meal_id,
	)
	return readMealReviewLine(row)
}
// Returns all of the pics and the pics associated with the host
// Use only for published meals
func GetAllPicsForMeal(db *sql.DB, meal_id int64) ([]*Pic, error) {
	meal_pics, err := GetMealPics(db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, err		
	}
	meal, err := GetMealById(db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, err		
	}
	host_pics, err := getHostPics(db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return nil, err		
	}
	return append(meal_pics, host_pics...), nil
}

func GetMealPics(db *sql.DB, meal_id int64) ([]*Pic, error) {
	rows, err := db.Query(`
		SELECT Name, Caption
		FROM MealPic
		WHERE Meal_id = ?`, meal_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readPicLines(rows)
}

func getHostPics(db *sql.DB, host_id int64) ([]*Pic, error) {
	rows, err := db.Query(`
		SELECT Name, Caption
		FROM HostPic
		WHERE Host_id = ?`, host_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readPicLines(rows)
}

func GetStripeTokenByGuestIdAndLast4(db *sql.DB, guest_id int64, last4 int64) (*StripeToken, error) {
	row := db.QueryRow(`
		SELECT Id, Stripe_token, Guest_id, Last4
		FROM StripeToken
		WHERE Guest_id = ? AND Last4 = ?`,
		guest_id,
		last4)
	return readStripeTokenLine(row)
}

func GetLast4sForGuest(db *sql.DB, guest_id int64) ([]int64, error) {
	rows, err := db.Query(`
		SELECT Last4
		FROM StripeToken
		WHERE Guest_id = ?`, guest_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	last4s := make([]int64, 0)
	for rows.Next() {
		var last4 int64
		if err := rows.Scan(
			&last4,
		); err != nil {
			return nil, err
		}
		if !contains(last4s, last4) { // protect against duplicates
			last4s = append(last4s, last4)
		}
	}
	return last4s, nil
}

func contains(s []int64, e int64) bool {
    for _, a := range s {
        if a == e {
            return true
        }
    }
    return false
}

func CreateHost(db *sql.DB, guest_id int64) error {
	_, err := db.Exec(`INSERT INTO Host
			(Guest_id)
			VALUES
			(?)`,
			guest_id,)
	return err
}

func UpdateGuest(db *sql.DB, first_name string, last_name string, email string, phone string, guest_id int64) error{
	_, err := db.Exec(`
		UPDATE Guest
		SET First_name = ?, Last_name = ?, Email = ?, Phone = ?
		WHERE Id =?`,
		first_name, last_name, email, phone, guest_id,
	)
	return err
}

func UpdateHost(db *sql.DB, address string, host_id int64) error{
	_, err := db.Exec(`
		UPDATE Host
		SET Address = ?
		WHERE Id =?`,
		address, host_id,
	)
	return err
}


func UpdateMealRequest(db *sql.DB, request_id int64, status int64) error {
	_, err := db.Exec(`
		UPDATE MealRequest
		SET Status = ?
		WHERE Id = ?`,
		status, request_id,
	)
	return err
}

func UpdateGuestFbToken(db *sql.DB, fb_id string, fb_token string) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Facebook_long_token = ?
		WHERE Facebook_id =?`,
		fb_token, fb_id,
	)
	return err
}

func UpdatePhoneForGuest(db *sql.DB, phone string, guest_id int64) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Phone = ?
		WHERE Id =?`,
		phone, guest_id,
	)
	return err

}

func UpdateEmailForGuest(db *sql.DB, email string, guest_id int64) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Email = ?
		WHERE Id =?`,
		email, guest_id,
	)
	return err
}

func UpdateMeal(db *sql.DB, meal_draft *Meal) error {
	_, err := db.Exec(
		`UPDATE Meal
		SET Host_id = ?, Price = ?, Title = ?, Description = ?, Capacity = ?, Starts = ?, Rsvp_by = ?
		WHERE Id = ?
		`,
		meal_draft.Host_id,
		meal_draft.Price,
		meal_draft.Title,
		meal_draft.Description,
		meal_draft.Capacity,
		meal_draft.Starts,
		meal_draft.Rsvp_by,
		meal_draft.Id,
	)
	return err
}

func readMealReviewLine(row *sql.Row) (*Review, error) {
	review := new(Review)
	if err := row.Scan(
		&review.Id,
		&review.Guest_id,
		&review.Rating,
		&review.Comment,
		&review.Meal_id,
		&review.Date,
		&review.Tip_percent,
	); err != nil {
		return nil, err
	}
	return review, nil
}

func readUserLine(row *sql.Row) (*UserData, error) {
	user_data := new(UserData)
	if err := row.Scan(
		&user_data.Id,
		&user_data.Email,
		&user_data.First_name,
		&user_data.Last_name,
		&user_data.password_salt,
		&user_data.password_hash,
		&user_data.password_reset_key,
		&user_data.Phone,
		&user_data.Verified,
	); err != nil {
		return nil, err
	}

	return user_data, nil
}

func readGuestLine(row *sql.Row) (*GuestData, error) {
	guest_data := new(GuestData)
	if err := row.Scan(
		&guest_data.Id,
		&guest_data.Email,
		&guest_data.First_name,
		&guest_data.Last_name,
		&guest_data.Facebook_id,
		&guest_data.Facebook_long_token,
		&guest_data.Phone,
	); err != nil {
		return nil, err
	}
	return guest_data, nil
}

func readStripeTokenLine(row *sql.Row) (*StripeToken, error) {
	stripe_token := new(StripeToken)
	if err := row.Scan(
		&stripe_token.Id,
		&stripe_token.Stripe_token,
		&stripe_token.Guest_id,
		&stripe_token.Last4,
	); err != nil {
		return nil, err
	}
	return stripe_token, nil
}

func readHostLine(row *sql.Row) (*HostData, error) {
	host_data := new(HostData)
	if err := row.Scan(
		&host_data.Id,
		&host_data.Guest_id,
		&host_data.Address,
		&host_data.Stripe_user_id,
		&host_data.Stripe_access_token,
		&host_data.Stripe_refresh_token,
		&host_data.Bio,
	); err != nil {
		return nil, err
	}
	return host_data, nil
}

func readMealLine(row *sql.Row) (*Meal, error) {
	meal := new(Meal)
	if err := row.Scan(
		&meal.Id,
		&meal.Host_id,
		&meal.Price,
		&meal.Title,
		&meal.Description,
		&meal.Capacity,
		&meal.Starts,
		&meal.Rsvp_by,
		&meal.Processed,
		&meal.Published, 
	); err != nil {
		return nil, err
	}
	return meal, nil
}

func readMealRequestLine(row *sql.Row) (*MealRequest, error) {
	meal_req := new(MealRequest)
	if err := row.Scan(
		&meal_req.Id,
		&meal_req.Guest_id,
		&meal_req.Meal_id,
		&meal_req.Seats,
		&meal_req.Status,
		&meal_req.Last4,
		&meal_req.Nudge_count,
		&meal_req.Last_nudge,
	); err != nil {
		return nil, err
	}
	return meal_req, nil
}

func readPicLines(rows *sql.Rows) ([]*Pic, error) {
	pics := make([]*Pic, 0)
	for rows.Next() {
		pic := new(Pic)
		if err := rows.Scan(
			&pic.Name,
			&pic.Caption,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		pics = append(pics, pic)
	}
	return pics, nil
}

func GetTruckById(db *sql.DB, id int64) (*Truck, error) {
	row := db.QueryRow(`SELECT Id, Owner, Name, Location_lat, Location_lon,
		Open_from, Open_until, Phone, Description FROM Truck
		WHERE Id = ?`, id)
	t := new(Truck)
	if err := row.Scan(
		&t.Id,
		&t.Owner,
		&t.Name,
		&t.Location_lat,
		&t.Location_lon,
		&t.Open_from,
		&t.Open_until,
		&t.Phone,
		&t.Description,
	); err != nil {
		return nil, err
	}
	return t, nil
}

func GetTrucksNearLocation(db *sql.DB, lat, lon float64, radius float64, open_from, open_til time.Time) ([]*Truck, error) {
	// Speed up the query a bit by doing a rough narrow before calculating
	// all the distances we might not need

	/*
		//TODO: Correct calculations for the bounding box
		rlat_min := math.Max(lat-2, -90.0)
		rlat_max := math.Min(lat+2, 90.0)
		rlon_min := math.Max(lon-2, -180.0)
		rlon_max := math.Min(lon+2, 180.0)
		WHERE Location_lat BETWEEN ? AND ?
		AND   Location_lon BETWEEN ? AND ?
	*/

	rows, err := db.Query(`
		SELECT Id, Name, Location_lat, Location_lon, Open_from, Open_until, Phone, Description,
		( 3959 * acos( cos( radians(?) )
               * cos( radians( Location_lat ) )
               * cos( radians( Location_lon ) - radians(?) )
               + sin( radians(?) )
               * sin( radians( Location_lat ) ) ) ) AS Distance
		FROM Truck
		HAVING Distance < ?
		AND (
			(Open_from <= ? AND Open_until >= ?)
			OR
			(Open_from <= ? AND Open_until >= ?)
			OR
			(Open_from >= ? AND Open_until <= ?)
			OR
			(Open_from <= ? AND Open_until >= ?)
		)
		ORDER BY Distance`,
		lat, lon, lat, radius, open_from, open_from, open_til, open_til,
		open_from, open_til, open_from, open_til,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trucks := make([]*Truck, 0)
	for rows.Next() {
		truck := new(Truck)
		if err := rows.Scan(
			&truck.Id,
			&truck.Name,
			&truck.Location_lat,
			&truck.Location_lon,
			&truck.Open_from,
			&truck.Open_until,
			&truck.Phone,
			&truck.Description,
			&truck.Distance,
		); err != nil {
			return nil, err
		}
		truck.Menus, err = GetMenusForTruck(db, truck.Id)
		if err != nil {
			return nil, err
		}
		if truck.Open_until.After(time.Now()) {
			truck.Open_now = true
		} else {
			truck.Open_now = false
		}
		trucks = append(trucks, truck)
	}
	return trucks, nil
}

func GetMenusForTruck(db *sql.DB, truck_id int64) ([]*Menu, error) {
	rows, err := db.Query(`
		SELECT Id, Truck_id, Name, Description, Flagship
		FROM Menu
		WHERE Truck_id = ?`, truck_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	menus := make([]*Menu, 0)
	for rows.Next() {
		menu := new(Menu)
		if err := rows.Scan(
			&menu.Id,
			&menu.Truck_id,
			&menu.Name,
			&menu.Description,
			&menu.Flagship,
		); err != nil {
			return nil, err
		}
		menu.Items, err = GetItemsForMenu(db, menu)
		if err != nil {
			return nil, err
		}
		menus = append(menus, menu)
	}
	return menus, nil
}

func GetItemsForMenu(db *sql.DB, menu *Menu) ([]*MenuItem, error) {
	rows, err := db.Query(`
		SELECT Id, Menu_id, Name, Price, Description, Pic_url
		FROM MenuItem
		WHERE Menu_id = ?`, menu.Id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuItem, 0)
	for rows.Next() {
		item := new(MenuItem)
		if err := rows.Scan(
			&item.Id,
			&item.Menu_id,
			&item.Name,
			&item.Price,
			&item.Description,
			&item.Pic_url,
		); err != nil {
			return nil, err
		}
		item.Truck_id = menu.Truck_id
		item.ListOptions, err = GetOptionsForMenuItem(db, item.Id)
		if err != nil {
			return nil, err
		}
		item.ToggleOptions, err = GetToggleOptionsForMenuItem(db, item.Id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func GetToggleOptionById(db *sql.DB, item_id int64) (*MenuToggleOption, error) {
	row := db.QueryRow(`
		SELECT Id, Item_id, Name, Price_modifier
		FROM MenuItemToggle
		WHERE Id = ?`, item_id)

	item := new(MenuToggleOption)
	if err := row.Scan(
		&item.Id,
		&item.Item_id,
		&item.Name,
		&item.Price_modifier,
	); err != nil {
		return nil, err
	}

	return item, nil
}

func GetToggleOptionsForMenuItem(db *sql.DB, item_id int64) ([]*MenuToggleOption, error) {
	rows, err := db.Query(`
		SELECT Id, Item_id, Name, Price_modifier
		FROM MenuItemToggle
		WHERE Item_id = ?`, item_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuToggleOption, 0)
	for rows.Next() {
		item := new(MenuToggleOption)
		if err := rows.Scan(
			&item.Id,
			&item.Item_id,
			&item.Name,
			&item.Price_modifier,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func GetOptionsForMenuItem(db *sql.DB, item_id int64) ([]*MenuItemOption, error) {
	rows, err := db.Query(`
		SELECT Id, Item_id, Name
		FROM MenuItemOption
		WHERE Item_id = ?`, item_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuItemOption, 0)
	for rows.Next() {
		item := new(MenuItemOption)
		if err := rows.Scan(
			&item.Id,
			&item.Item_id,
			&item.Name,
		); err != nil {
			return nil, err
		}
		item.Values, err = GetOptionValuesForMenuItem(db, item.Id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil

}

func GetMenuItemById(db *sql.DB, item_id int64) (*MenuItem, error) {
	row := db.QueryRow(`
		SELECT Id, Menu_id, Name, Price, Description
		FROM MenuItem
		WHERE Id = ?`, item_id)

	item := new(MenuItem)
	if err := row.Scan(
		&item.Id,
		&item.Menu_id,
		&item.Name,
		&item.Price,
		&item.Description,
	); err != nil {
		return nil, err
	}

	var err error
	item.ListOptions, err = GetOptionsForMenuItem(db, item.Id)
	if err != nil {
		return nil, err
	}

	item.ToggleOptions, err = GetToggleOptionsForMenuItem(db, item.Id)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func GetListOptionValueById(db *sql.DB, option_id int64) (*MenuItemOptionItem, error) {
	row := db.QueryRow(`
		SELECT Id, Option_id, Option_name, Price_modifier
		FROM MenuItemOptionItem
		WHERE Id = ?`, option_id,
	)
	item := new(MenuItemOptionItem)
	if err := row.Scan(
		&item.Id,
		&item.Option_id,
		&item.Option_name,
		&item.Price_modifier,
	); err != nil {
		return nil, err
	}
	return item, nil
}

func GetOptionValuesForMenuItem(db *sql.DB, option_id int64) ([]*MenuItemOptionItem, error) {
	rows, err := db.Query(`
		SELECT Id, Option_id, Option_name, Price_modifier
		FROM MenuItemOptionItem
		WHERE Option_id = ?`, option_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*MenuItemOptionItem, 0)
	for rows.Next() {
		item := new(MenuItemOptionItem)
		if err := rows.Scan(
			&item.Id,
			&item.Option_id,
			&item.Option_name,
			&item.Price_modifier,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil

}

func SaveOrderToDB(db *sql.DB, order *Order) (int64, error) {
	// Insert top level order item  
	result, err := db.Exec(
		"INSERT INTO `Order`"+
			`(User_id, Truck_id, Date, Pickup_time)
		VALUES (?, ?, ?, ?)`,
		order.User_id, order.Truck_id, order.Date, order.Pickup_time)
	if err != nil {
		return 0, err
	}
	order.Id, err = result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Insert each of the items in the order
	// For each item, also insert mappings for the toggle / list options
	for _, item := range order.Items {
		result, err = db.Exec(
			`INSERT INTO OrderItem
			(Order_id, Item_id, Quantity)
			VALUES
			(?, ?, ?)`,
			order.Id, item.Item_id, item.Quantity)
		if err != nil {
			return 0, err
		}
		item.Id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}

		for _, toggle_value_id := range item.ToggleOptions {
			_, err := db.Exec(
				`INSERT INTO OrderToggledOptions
				(Order_item_id, Toggle_value_id)
				VALUES
				(?, ?)`,
				item.Id, toggle_value_id)
			if err != nil {
				return 0, err
			}
		}

		for _, list_value_id := range item.ListOptionValues {
			_, err := db.Exec(
				`INSERT INTO OrderSelectedOptions
				(Order_item_id, Value_id)
				VALUES
				(?, ?)`,
				item.Id, list_value_id)
			if err != nil {
				return 0, err
			}
		}
	}

	return order.Id, nil
}

func SavePic(db *sql.DB, pic_name string, caption string, meal_id int64) error {
	result, err := db.Exec(
		`INSERT INTO MealPic
		(Name, Caption, Meal_id)
		VALUES
		(?, ?, ?)
		`,
		pic_name,
		caption,
		meal_id,
	)
	log.Println("Last inserted row: %d", result.LastInsertId())
	return err
}

func CreateMeal(db *sql.DB, meal_draft *Meal) (sql.Result, error) {
	return db.Exec(
		`INSERT INTO Meal
		(Host_id, Price, Title, Description, Capacity, Starts, Rsvp_by)
		VALUES
		(?, ?, ?, ?, ?, ?, ?)
		`,
		meal_draft.Host_id,
		meal_draft.Price,
		meal_draft.Title,
		meal_draft.Description,
		meal_draft.Capacity,
		meal_draft.Starts,
		meal_draft.Rsvp_by,
	)
}

func SaveReview(db *sql.DB, guest_id int64, meal_id int64, rating int64, comment string, tip_percent int64) error {
	_, err := db.Exec(
		`INSERT INTO HostReview
		(Guest_id, Meal_id, Rating, Comment, Date, Tip_percent)
		VALUES
		(?, ?, ?, ?, ?, ?)
		`,
		guest_id,
		meal_id,
		rating,
		comment,
		time.Now(),
		tip_percent,
	)
	return err
}

func SavePaymentToken(db *sql.DB, token *PaymentToken) error {
	_, err := db.Exec(
		`INSERT INTO PaymentToken
		(User_id, Name, Stripe_key, Token)
		VALUES
		(?, ?, ?, ?)
		`,
		token.User_id,
		token.Name,
		token.stripe_key,
		token.Token,
	)
	return err
}

// TODO: seats cannot exceed available spots
// Just another test...
func SaveMealRequest(db *sql.DB, meal_req *MealRequest) error {
	_, err := db.Exec(
		`INSERT INTO MealRequest
		(Guest_id, Meal_id, Seats, Status, Last4)
		VALUES
		(?, ?, ?, ?, ?)
		`,
		meal_req.Guest_id,
		meal_req.Meal_id,
		meal_req.Seats,
		0,
		meal_req.Last4,
	)
	return err
}

func SaveStripeToken(db *sql.DB, stripe_token string, last4 int64, guest_data *GuestData) error {
	stripe.Key = "***REMOVED***"

	customerParams := &stripe.CustomerParams{
		Desc:  guest_data.First_name + " " + guest_data.Last_name,
		Email: guest_data.Email,
	}
	log.Println(stripe_token)
	customerParams.SetSource(stripe_token) // obtained with Stripe.js
	c, err := customer.New(customerParams)
	if err != nil {
		log.Println(err)
		return err
	}
	log.Println(c)
	_, err = db.Exec(`
		INSERT INTO StripeToken
		(Stripe_token, Last4, Guest_id)
		VALUES
		(?, ?, ?)
		`,
		c.ID, 
		last4, 
		guest_data.Id,
	)
	if err != nil {
		log.Println(err)
	}
	return err
}

// Should only be called with a successful stripe response
func UpdateStripeConnect(db *sql.DB, stripe_response map[string]interface {}, host_id int64) error {
	stripe_user_id := stripe_response["stripe_user_id"].(string)
	access_token := stripe_response["access_token"].(string)
	refresh_token := stripe_response["refresh_token"].(string)
	_, err := db.Exec(`
		UPDATE Host
		SET Stripe_user_id = ?,
			Stripe_access_token = ?,
			Stripe_refresh_token = ?
		WHERE Id = ?
		`,
		stripe_user_id, 
		access_token, 
		refresh_token, 
		host_id,
	)

	return err
}

func GetPaymentToken(db *sql.DB, token_uuid string) (*PaymentToken, error) {
	row := db.QueryRow(`SELECT Id, User_id, Name, Stripe_key, Token, Created
        FROM PaymentToken WHERE Token = ?`, token_uuid)
	payment_data := new(PaymentToken)
	if err := row.Scan(
		&payment_data.Id,
		&payment_data.User_id,
		&payment_data.Name,
		&payment_data.stripe_key,
		&payment_data.Token,
		&payment_data.Created,
	); err != nil {
		return nil, err
	}
	return payment_data, nil
}

func GetTrucksForOwner(db *sql.DB, user *UserData) ([]*Truck, error) {
	rows, err := db.Query(`
		SELECT Id, Name, Location_lat, Location_lon, Open_from, Open_until, Phone, Description
		FROM Truck
		WHERE Owner = ?`,
		user.Id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trucks := make([]*Truck, 0)
	for rows.Next() {
		truck := new(Truck)
		if err := rows.Scan(
			&truck.Id,
			&truck.Name,
			&truck.Location_lat,
			&truck.Location_lon,
			&truck.Open_from,
			&truck.Open_until,
			&truck.Phone,
			&truck.Description,
		); err != nil {
			return nil, err
		}
		trucks = append(trucks, truck)
	}
	return trucks, nil
}

func SetOwnerForTruck(db *sql.DB, truck_id int64, user_id int64) error {
	_, err := db.Exec(`
		UPDATE Truck
		SET Owner = ?
		WHERE Id = ?`,
		user_id, truck_id,
	)
	return err
}
