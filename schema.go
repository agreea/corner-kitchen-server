package main

import (
	"database/sql"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
	"time"
	"log"
	"errors"
	"io/ioutil"
	"encoding/base64"
	"syscall"
	"strings"
	"code.google.com/p/go-uuid/uuid"
	"os"
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
	First_name          string
	Last_name			string
	Facebook_id    		string
	Facebook_long_token	string
	Prof_pic 			string
	Bio 				string

	// Go fields
	Session_token 		string
	Is_host 			bool
	Phone 				string
	Email          		string
	Phone_verified 		bool
	Email_verified 		bool
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
	Processed 		int64
	Published 		int64
	// go fields
	Pics 			[]*Pic
	Popups 			[]*Popup
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
	City 					string
	State 					string
	Stripe_user_id 			string
	Stripe_access_token		string
	Stripe_refresh_token	string
	Bio						string
}

type AttendeeData struct {
	Guest 		*GuestData
	Seats 		int64
}

type PopupBooking struct {
	Id       		int64
	Guest_id 		int64
	Popup_id  		int64
	Seats 	 		int64
	Last4 	 		int64
	Nudge_count 	int64
	Meal_price 		float64
	Last_nudge 		time.Time
}

type Review struct {
	Id 			int64
	Guest_id 	int64
	Rating 		int64
	Comment 	string
	Popup_id 	int64
	Date 		time.Time
	Tip_percent int64
	Suggestion 	string
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
	row := db.QueryRow(`SELECT Id, First_name, Last_name,
		Facebook_id, Facebook_long_token, Prof_pic, Bio
		FROM Guest WHERE Facebook_id = ?`, fb_id)
	return readGuestLine(row)
}

func GetGuestById(db *sql.DB, id int64) (*GuestData, error) {
	row := db.QueryRow(`SELECT Id, First_name, Last_name,
		Facebook_id, Facebook_long_token, Prof_pic, Bio
		FROM Guest WHERE Id = ?`, id)
	return readGuestLine(row)
}

func GetGuestByEmail(db *sql.DB, email string) (*GuestData, error) {
	email_row := db.QueryRow(`SELECT Guest_id FROM GuestEmail WHERE Email = ?`, email)
	guest_id := 0
	err := email_row.Scan(&guest_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	log.Println(guest_id)
	return GetGuestById(db, int64(guest_id))
}

func GetGuestByHostId(db *sql.DB, host_id int64) (*GuestData, error) {
	guest_id_in_host_table := db.QueryRow(`SELECT Guest_id FROM Host WHERE Id = ?`, host_id)
	guest_id := 0
	err := guest_id_in_host_table.Scan(&guest_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return GetGuestById(db, int64(guest_id))
}

func GetFacebookPic(fb_id string) string {
	return "https://graph.facebook.com/" + fb_id + "/picture?width=200&height=200"
}

func GetPhonePinForGuest(db *sql.DB, guest_id int64) (int64, error) {
	pin := 0
	row := db.QueryRow("SELECT Pin FROM GuestPhone WHERE Guest_id = ?", guest_id)
	err := row.Scan(&pin)
	return int64(pin), err
}

func GetPhoneForGuest(db *sql.DB, guest_id int64) (string, error) {
	phone := "0"
	row := db.QueryRow("SELECT Phone FROM GuestPhone WHERE Guest_id = ?", guest_id)
	err := row.Scan(&phone)
	return phone, err
}

func VerifyPhoneForGuest(db *sql.DB, guest_id int64) (error) {
	_, err := db.Exec(`UPDATE GuestPhone SET Verified = 1 WHERE Guest_id = ?` , guest_id)
	return err
}
func GetPhoneStatus(db *sql.DB, guest_id int64) (bool, error) {
	verified := 0
	row := db.QueryRow("SELECT Verified FROM GuestPhone WHERE Guest_id = ?", guest_id)
	err := row.Scan(&verified)
	return (verified == 1), err
}
func GetEmailForGuest(db *sql.DB, guest_id int64) (string, error) {
	email := ""
	row := db.QueryRow("SELECT Email FROM GuestEmail WHERE Guest_id = ?", guest_id)
	err := row.Scan(&email)
	return email, err
}

func GetEmailCodeForGuest(db *sql.DB, guest_id int64) (string, error) {
	code := ""
	row := db.QueryRow("SELECT Code FROM GuestEmail WHERE Guest_id = ?", guest_id)
	err := row.Scan(&code)
	return code, err
}
func GetEmailStatus(db *sql.DB, guest_id int64) (bool, error) {
	verified := 0
	row := db.QueryRow("SELECT Verified FROM GuestEmail WHERE Guest_id = ?", guest_id)
	err := row.Scan(&verified)
	return (verified == 1), err
}
func VerifyEmailForGuest(db *sql.DB, guest_id int64) (error) {
	_, err := db.Exec(`UPDATE GuestEmail SET Verified = 1 WHERE Guest_id = ?` , guest_id)
	return err
}
func RecordFollowHost(db *sql.DB, guest_id, host_id int64) error {
	_, err := db.Exec(`INSERT INTO HostFollowers
			(Guest_id, Host_id)
			VALUES
			(?, ?)`,
			guest_id, host_id,)
	return err
}

func GetGuestFollowsHost(db *sql.DB, guest_id, host_id int64)(bool) {
	row := db.QueryRow("SELECT Id FROM HostFollowers WHERE Guest_id = ? AND Host_id = ?", 
		guest_id, 
		host_id)
	follow_id := 0
	log.Println("About to scan row")
	if err := row.Scan(&follow_id); err != nil {
		return false
	}
	return true
}

func GetHostByGuestId(db *sql.DB, guest_id int64) (*HostData, error) {
	log.Println(guest_id)
	row := db.QueryRow(`SELECT Id, Guest_id, Address, City, State,
		Stripe_user_id, Stripe_access_token, Stripe_refresh_token, Bio 
		FROM Host WHERE Guest_id = ?`, guest_id)
	return readHostLine(row)
}

func GetHostById(db *sql.DB, id int64) (*HostData, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Address, City, State,
		Stripe_user_id, Stripe_access_token, Stripe_refresh_token, Bio 
		FROM Host WHERE Id = ?`, id)
	return readHostLine(row)
}

func GetHostBySession(db *sql.DB, session_manager *SessionManager, session_id string) (*HostData, error) {
	valid, session, err := session_manager.GetGuestSession(session_id)
	if err != nil {
		return nil, errors.New("Couldn't locate guest")
	}
	if !valid {
		return nil, errors.New("Invalid session")
	}
	return GetHostByGuestId(db, session.Guest.Id)
}

func GetNewHostStatus(db *sql.DB, host_id int64) (bool, error) {
	total_revenue := float64(0)
	meals, err := GetMealsForHost(db, host_id)
	if err != nil {
		log.Println(err)
		return false, err
	}
	for _, meal := range meals {
		// for each meal: get all popups
		for _, popup := range meal.Popups { // for each popup, get the bookings
			bookings, err := GetBookingsForPopup(db, popup.Id)
			if err != nil {
				log.Println(err)
				return false, err
			}
			for _, booking := range bookings { // for each booking, add the revenue contributed to the total
				if booking.Last4 == 0 {
					continue
				}
				total_revenue += float64(booking.Seats) * meal.Price
				if total_revenue > 400 {
					return false, nil
				}
			}
		}
	}
	return true, nil
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
	row := db.QueryRow(`SELECT Id, Host_id, Price, Title, 
		Description, Processed, Published
        FROM Meal 
        WHERE Id = ?`, id)
	return readMealLine(row)
}

func GetMealByPopupId(db *sql.DB, popup_id int64) (*Meal, error) {
	popup, err := GetPopupById(db, popup_id)
	if err != nil {
		return nil, err
	}
	return GetMealById(db, popup.Meal_id)
}

func GetMealsForHost(db *sql.DB, host_id int64) ([]*Meal, error) {
	rows, err := db.Query(`SELECT Id, Host_id, Price, Title, Description, Processed, Published
        FROM Meal 
        WHERE Host_id = ?`, 
        host_id)
	if err != nil {
		log.Println(err)
		log.Println(host_id)
		return nil, err
	}
	defer rows.Close()
	meals, err := read_meal_rows(db, rows)
	if err != nil {
		return nil, err
	}
	for _, meal := range meals {
		meal.Popups, err = GetPopupsForMeal(db, meal.Id)
		if err != nil {
			log.Println("Problem is with popups")
			return nil, err
		}
	}
	return meals, nil
}

func read_meal_rows(db *sql.DB, rows *sql.Rows) ([]*Meal, error) {
	meals := make([]*Meal, 0)
	for rows.Next() {
		meal := new(Meal)
		if err := rows.Scan(
			&meal.Id,
			&meal.Host_id,
			&meal.Price,
			&meal.Title,
			&meal.Description,
			&meal.Processed,
			&meal.Published,
		); err != nil {
			return nil, err
		}
		meals = append(meals, meal)
	}
	return meals, nil
}

// avoids double-charging if both qa and prod are running chronjobs
func SetPopupProcessed(db *sql.DB, popup_id int64) error{
	_, err := db.Exec(`
		UPDATE Popup
		SET Processed = 1
		WHERE Id = ?
		`,
		popup_id,
	)
	if err != nil {
		log.Println(err)
	}
	return err
}

func GetMealReviewByGuestIdAndPopupId(db *sql.DB, guest_id int64, meal_id int64) (meal_review *Review, err error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Rating, Comment, Popup_id, Date, Tip_percent
        FROM HostReview
        WHERE Guest_id = ? AND Meal_id = ?`, guest_id, meal_id)
	return readReviewLine(row)
}

func GetUpcomingMealsFromDB(db *sql.DB) ([]*Meal_read, error) {
	rows, err := db.Query(`
		SELECT Id
		FROM Popup
		WHERE Rsvp_by > ? AND Id > 0
		ORDER BY Rsvp_by ASC`,
		time.Now(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meal_reads := make([]*Meal_read, 0)
	// get the guest for each guest id and add them to the slice of guests
	for rows.Next() {
		popup_id := 0
		if err := rows.Scan(
			&popup_id,
		); err != nil {
			return nil, err
		}
		popup, err := GetPopupById(db, int64(popup_id))
		log.Println(popup_id)
		if err != nil {
			return nil, err
		}
		if (meal_reads_contains(popup.Meal_id, meal_reads)) {
			continue
		}
		meal_read, err := GetMealCardDataById(db, popup.Meal_id)
		if err != nil {
			log.Println(err)
			continue
		}
		meal_reads = append(meal_reads, meal_read)
	}
	return meal_reads, nil
}
func meal_reads_contains(meal_id int64, meal_reads []*Meal_read) bool{
	for _, meal_read := range meal_reads {
		if meal_read.Id == meal_id {
			return true
		}
	}
	return false
}
// Returns meal objects with all fields necessary for display in a meal card
func GetMealCardDataById(db *sql.DB, meal_id int64) (*Meal_read, error) {
	meal, err := GetMealById(db, meal_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	meal_data := new(Meal_read)
	meal_data.Id = meal.Id
	meal_data.Title = meal.Title
	meal_data.Description = meal.Description
	meal_data.Price, err = GetMealPrice(db, meal)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	meal_data.Pics, err = GetAllPicsForMeal(db, meal.Id)
	if err != nil{ 
		log.Println(err)
		return nil, err
	}
	host, err := GetHostById(db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	host_as_guest, err := GetGuestById(db, host.Guest_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	meal_data.Host_name = host_as_guest.First_name
	if host_as_guest.Prof_pic != "" {
		meal_data.Host_pic = "https://yaychakula.com/img/" + host_as_guest.Prof_pic
	} else {
		meal_data.Host_pic = GetFacebookPic(host_as_guest.Facebook_id)
	}
	meal_data.Host_id = host.Id
	meal_data.Host_bio = host_as_guest.Bio
	meal_data.Popups, err = GetPopupsForMeal(db, meal.Id)
	meal_data.New_host, err = GetNewHostStatus(db, meal.Host_id)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return meal_data, nil
}

func GetPopupsForMeal(db *sql.DB, meal_id int64) ([]*Popup, error) {
	log.Println("popups for meal: ", meal_id)
	rows, err := db.Query(`SELECT Id, Meal_id, Starts, Rsvp_by, Address, City, State, Capacity, Processed
        FROM Popup 
        WHERE Meal_id = ?`, meal_id,
	)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer rows.Close()
	popups := make([]*Popup, 0)
	for rows.Next() { // construct each meal, get its reviews, and append them to all host reviews
		popup := new(Popup)
		if err := rows.Scan(
			&popup.Id,
			&popup.Meal_id,
			&popup.Starts,
			&popup.Rsvp_by,
			&popup.Address,
			&popup.City,
			&popup.State,
			&popup.Capacity,
			&popup.Processed,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		attendees, err := GetAttendeesForPopup(db, popup.Id)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		popup.Attendees, err = get_attendee_reads_for_attendees(attendees)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		full_address := popup.Address + ", " + popup.City + ", " + popup.State
		popup.Maps_url, err = GetStaticMapsUrlForMeal(db, full_address)
		if err != nil {
			log.Println(err)
			return nil, err
		}
		log.Println("Popup successfully appended: ", popup.Id)
		popups = append(popups, popup)
	}
	return popups, nil
}

func GetPopupById(db *sql.DB, popup_id int64) (*Popup, error) {
	row := db.QueryRow(`SELECT Id, Meal_id, Starts, Rsvp_by, Address, City, State, Capacity, Processed
        FROM Popup 
        WHERE Id = ?`, popup_id,
	)
	return readPopupLine(row)
}

func GetPopupsFromTimeWindow(db *sql.DB, window_starts, window_ends time.Time) ([]*Popup, error) {
	rows, err := db.Query(`SELECT Id, Meal_id, Starts, Rsvp_by, Address, City, State, Capacity, Processed
        FROM Popup 
        WHERE Starts > ? AND Starts < ?`, 
        window_starts,
        window_ends)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return read_popup_rows(rows)
}

func read_popup_rows(rows *sql.Rows) ([]*Popup, error) {
	popups := make([]*Popup, 0)
	for rows.Next() {
		popup := new(Popup)
		if err := rows.Scan(
			&popup.Id,
			&popup.Meal_id,
			&popup.Starts,
			&popup.Rsvp_by,
			&popup.Address,
			&popup.City,
			&popup.State,
			&popup.Capacity,
			&popup.Processed,
		); err != nil {
			return nil, err
		}
		popups = append(popups, popup)
	}
	return popups, nil
}


func CreatePopup(db *sql.DB, popup *Popup) (sql.Result, error) {
	return db.Exec(
		`INSERT INTO Popup
		(Meal_id, Capacity, Starts, Rsvp_by, Address, City, State, Processed)
		VALUES
		(?, ?, ?, ?, ?, ?, ?, 0)
		`,
		popup.Meal_id,
		popup.Capacity,
		popup.Starts,
		popup.Rsvp_by,
		popup.Address,
		popup.City,
		popup.State,
	)
}
func GetBookingById(db *sql.DB, booking_id int64) (*PopupBooking, error) {
	row := db.QueryRow(`
		SELECT Id, Popup_id, Guest_id, Seats, Last4, Nudge_count, Last_nudge, Meal_price
        FROM PopupBooking 
        WHERE Id = ?`, booking_id,
	)
	return readBookingLine(row)
}

func GetBookingsForPopup(db *sql.DB, popup_id int64) ([]*PopupBooking, error) {
	rows, err := db.Query(`
		SELECT Id, Popup_id, Guest_id, Seats, Last4, Nudge_count, Last_nudge, Meal_price
		FROM PopupBooking
		WHERE Popup_id = ?`, popup_id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bookings := make([]*PopupBooking, 0)
	for rows.Next() {
		booking := new(PopupBooking)
		if err := rows.Scan(
			&booking.Id,
			&booking.Popup_id,
			&booking.Guest_id,
			&booking.Seats,
			&booking.Last4,
			&booking.Nudge_count,
			&booking.Last_nudge,
			&booking.Meal_price,
		); err != nil {
			return nil, err
		}
		bookings = append(bookings, booking)
	}
	return bookings, nil	
}
func GetBookingByGuestAndPopupId(db *sql.DB, guest_id, popup_id int64) (*PopupBooking, error) {
	row := db.QueryRow(`SELECT Id, Popup_id, Guest_id, Seats, Last4, Nudge_count, Last_nudge, Meal_price
        FROM PopupBooking 
        WHERE Guest_id = ? AND Popup_id = ?`, guest_id, popup_id,
	)
	return readBookingLine(row)
}

func SavePopupBooking(db *sql.DB, booking *PopupBooking) error {
	_, err := db.Exec(
		`INSERT INTO PopupBooking
		(Guest_id, Popup_id, Seats, Last4, Nudge_count, Last_nudge, Meal_price)
		VALUES
		(?, ?, ?, ?, ?, ?, ?)`,
		booking.Guest_id, 
		booking.Popup_id,
		booking.Seats, 
		booking.Last4, 
		booking.Nudge_count, 
		time.Now(),
		booking.Meal_price)
	return err
}

func get_attendee_reads_for_attendees(attendees []*AttendeeData) ([]*Attendee_read, error) {
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
    	attendee_reads = append(attendee_reads, attendee_read)
	}
	return attendee_reads, nil
}


func GetAttendeesForPopup(db *sql.DB, popup_id int64) ([]*AttendeeData, error) {
	// get all the guest ids attending the meal
	rows, err := db.Query(`
		SELECT Guest_id, Seats
		FROM PopupBooking
		WHERE Popup_id = ?`, popup_id,
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

func GetReviewById(db *sql.DB, review_id int64) (*Review, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Rating, Comment, Popup_id, Date, Tip_percent, Suggestion
        FROM HostReview
        WHERE Id = ?`, review_id)
	return readReviewLine(row)
}

/*
UPDATE HostReview
SET "Meal_id" = (SELECT "Id" FROM Popup WHERE "Meal_id" = (SELECT "Meal_id" FROM HostReview))
*/
func GetReviewsForHost(db *sql.DB, host_id int64) ([]*Review, error) {
	// get all meal ids associated with host
	rows, err := db.Query(`SELECT Id FROM Meal WHERE Host_id = ?`, host_id,)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer rows.Close()
	host_reviews := make([]*Review, 0)
	for rows.Next() { // for each meal, get all associated popups.
		meal_id := int64(0)
		if err := rows.Scan(
			&meal_id,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		popups, err := GetPopupsForMeal(db, meal_id) 
		if err != nil {
			log.Println(err)
			return nil, err
		}
		for _, popup := range popups { // for each popup, get all associated reviews
			popup_reviews, err := GetReviewsForPopup(db, popup.Id) 
			if err != nil {
				log.Println(err)
				continue
			}
			host_reviews = append(host_reviews, popup_reviews...)
		}
	}
	return host_reviews, nil
}

func GetReviewsForPopup(db *sql.DB, popup_id int64) ([]*Review, error) {
	rows, err := db.Query(`
		SELECT Id, Guest_id, Rating, Comment, Popup_id, Date, Tip_percent, Suggestion
		FROM HostReview
		WHERE Popup_id = ?`,
		popup_id,
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
			&review.Popup_id,
			&review.Date,
			&review.Tip_percent,
			&review.Suggestion,
		); err != nil {
			log.Println(err)
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func GetReviewByGuestAndPopupId(db *sql.DB, guest_id int64, meal_id int64) (*Review, error) {
	row := db.QueryRow(`
		SELECT Id, Guest_id, Rating, Comment, Popup_id, Date, Tip_percent, Suggestion
		FROM HostReview
		WHERE Guest_id = ? AND Meal_id = ?`,
		guest_id,
		meal_id,
	)
	return readReviewLine(row)
}

func StoreLocation(db *sql.DB, lat, lng float64, full_address, polyline string) error {
	_, err := db.Exec(`INSERT INTO Location
			(Address, Lat, Lng, Polyline)
			VALUES
			(?, ?, ?, ?)`,
			full_address, lat, lng, polyline)
	return err
}

func GetLocationByAddress(db *sql.DB, full_address string) (*Location, error) {
	location := new(Location)
	row := db.QueryRow(`
		SELECT Lat, Lng, Polyline FROM Location WHERE Address = ?`, full_address)
	if err := row.Scan(&location.Lat, &location.Lng, &location.Polyline); err != nil {
		return nil, err
	}
	return location, nil
}

func GetStaticMapsUrlForMeal(db *sql.DB, full_address string) (string, error) {
	log.Println(full_address)
	row := db.QueryRow(`
		SELECT Polyline FROM Location WHERE Address = ?`, full_address)
	polyline := ""
	if err := row.Scan(&polyline,); err != nil {
		return "", err
	}
	maps_url := 
		"https://maps.googleapis.com/maps/api/staticmap?" +
		"size=600x400&zoom=14&path=fillcolor:0x5BC0DEa9|enc:" +
		polyline
	return maps_url, nil
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

func UpdateGuest(db *sql.DB, first_name, last_name, bio string, guest_id int64) error{
	_, err := db.Exec(`
		UPDATE Guest
		SET First_name = ?, Last_name = ?, Bio = ?
		WHERE Id =?`,
		first_name, last_name, bio, guest_id,
	)
	return err
}

func UpdateHost(db *sql.DB, address, city, state string, host_id int64) error{
	_, err := db.Exec(`
		UPDATE Host
		SET Address = ?, City = ?, State = ?
		WHERE Id =?`,
		address, 
		city, 
		state, 
		host_id,
	)
	return err
}

func SaveProfPic(db *sql.DB, file_name string, guest_id int64) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Prof_pic = ? WHERE Id = ?`,
		file_name,
		guest_id,
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


func UpdatePhone(db *sql.DB, phone string, pin, guest_id int64) error {
	_, err := GetPhonePinForGuest(db, guest_id)
	if err != nil {
		_, err := db.Exec(
			`INSERT INTO GuestPhone
			(Phone, Pin, Guest_id)
			VALUES
			(?, ?, ?)
			`,
			phone,
			pin,
			guest_id,
		)
		return err
	}
	_, err = db.Exec(`
		UPDATE GuestPhone
		SET Phone = ?, Pin = ?, Verified = 0
		WHERE Guest_id = ?`,
		phone, pin, guest_id,
	)
	return err
}

func UpdateEmail(db *sql.DB, email, code string, guest_id int64) error {
	_, err := GetEmailCodeForGuest(db, guest_id)
	if err != nil {
		_, err := db.Exec(
			`INSERT INTO GuestEmail
			(Email, Code, Guest_id)
			VALUES
			(?, ?, ?)
			`,
			email,
			code,
			guest_id,
		)
		MailChimpRegister(email, false, db) // subscribe them to Mailchimp if they haven't already
		return err
	}
	_, err = db.Exec(`
		UPDATE GuestEmail
		SET Email = ?, Code = ?
		WHERE Guest_id = ?`,
		email, code, guest_id,
	)
	return err
}

func UpdateBio(db *sql.DB, bio string, guest_id int64) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Bio = ?
		WHERE Id = ?`,
		bio, guest_id,
	)
	return err
}
func UpdateFb(db *sql.DB, token string, fb_id, guest_id int64) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Facebook_long_token = ?, Facebook_id = ?
		WHERE Id = ?`,
		token, fb_id, guest_id,
	)
	return err
}
func UpdateMeal(db *sql.DB, meal_draft *Meal) error {
	_, err := db.Exec(
		`UPDATE Meal
		SET Host_id = ?, Price = ?, Title = ?, Description = ?
		WHERE Id = ?
		`,
		meal_draft.Host_id,
		meal_draft.Price,
		meal_draft.Title,
		meal_draft.Description,
		meal_draft.Id,
	)
	return err
}

func readReviewLine(row *sql.Row) (*Review, error) {
	review := new(Review)
	if err := row.Scan(
		&review.Id,
		&review.Guest_id,
		&review.Rating,
		&review.Comment,
		&review.Popup_id,
		&review.Date,
		&review.Tip_percent,
		&review.Suggestion,
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

func readPopupLine(row *sql.Row) (*Popup, error) {
	popup := new(Popup)
	if err := row.Scan(
		&popup.Id,
		&popup.Meal_id,
		&popup.Starts,
		&popup.Rsvp_by,
		&popup.Address,
		&popup.City,
		&popup.State,
		&popup.Capacity,
		&popup.Processed,
	); err != nil {
		return nil, err
	}
	log.Println("Got the popup just fine")
	return popup, nil
}

func readBookingLine(row *sql.Row) (*PopupBooking, error) {
	booking := new(PopupBooking)
	if err := row.Scan(
		&booking.Id,
		&booking.Popup_id,
		&booking.Guest_id,
		&booking.Seats,
		&booking.Last4,
		&booking.Nudge_count,
		&booking.Last_nudge,
		&booking.Meal_price,
	); err != nil {
		return nil, err
	}
	return booking, nil
}


func readGuestLine(row *sql.Row) (*GuestData, error) {
	guest_data := new(GuestData)
	if err := row.Scan(
		&guest_data.Id,
		&guest_data.First_name,
		&guest_data.Last_name,
		&guest_data.Facebook_id,
		&guest_data.Facebook_long_token,
		&guest_data.Prof_pic,
		&guest_data.Bio,
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
		&host_data.City,
		&host_data.State,
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
		&meal.Processed,
		&meal.Published, 
	); err != nil {
		return nil, err
	}
	return meal, nil
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

func SaveMealPic(db *sql.DB, pic_name string, caption string, meal_id int64) error {
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
	id, err := result.LastInsertId()
	log.Println("Last inserted row: %d", id)
	return err
}

func CreateMeal(db *sql.DB, meal_draft *Meal) (sql.Result, error) {
	return db.Exec(
		`INSERT INTO Meal
		(Host_id, Price, Title, Description)
		VALUES
		(?, ?, ?, ?)
		`,
		meal_draft.Host_id,
		meal_draft.Price,
		meal_draft.Title,
		meal_draft.Description,
	)
}

func SaveReview(db *sql.DB, review *Review) error {
	_, err := db.Exec(
		`INSERT INTO HostReview
		(Guest_id, Popup_id, Rating, Comment, Suggestion, Date, Tip_percent)
		VALUES
		(?, ?, ?, ?, ?, ?, ?)
		`,
		review.Guest_id,
		review.Popup_id,
		review.Rating,
		review.Comment,
		review.Suggestion,
		time.Now(),
		review.Tip_percent,
	)
	if err == nil {
		_, err := db.Exec(`
			UPDATE PopupBooking
			SET Nudge_count = -1
			WHERE Guest_id = ? AND Popup_id = ?
			`,
			review.Guest_id,
			review.Popup_id,
		)
		return err
	}
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

func SaveStripeToken(db *sql.DB, stripe_token string, last4 int64, guest_data *GuestData) error {
	if (server_config.Version.V == "prod") {
		stripe.Key = "***REMOVED***"
	} else {
		stripe.Key = "***REMOVED***"
	}
	email, err := GetEmailForGuest(db, guest_data.Id)
	if err != nil {
		return err
	}
	guest_data.Email = email
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


// Global Utility Functions:

func CreatePicFile(pic_as_string string) (string, error) {
	pic_s_split := strings.Split(string(pic_as_string), "base64,")
	data, err := base64.StdEncoding.DecodeString(pic_s_split[1])
	if err != nil {
		return "", err
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
		return "", err
	} else {
		file, err := os.Open(file_address)
     	if err != nil {
         // handle the error here
         return "", err
     	}
     	defer file.Close()
	   stat, err := file.Stat()
	   if err != nil {
	       return "", err
	   }
	   log.Println(stat)
	}
	return file_name, nil
}