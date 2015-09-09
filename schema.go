package main

import (
	"database/sql"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
	"time"
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
	Name           		string
	Facebook_id    		string
	Stripe_cust_id 		string
	Facebook_long_token	string
	// Go fields
	Session_token 		string
}

type FacebookResp struct {
	Id    string
	Email string
	Name  string
}

type KitchenSession struct {
	Guest   *GuestData
	Expires time.Time
}

type Meal struct {
	Id      int64
	Host_id int64
	Price   float64
	Title   string
}

type StripeToken struct {
	Id        int64
	Guest_id  int64
	Stripe_id int64
}

type HostData struct {
	Id             int64
	Guest_id       int64
	Address        string
	Phone          string
	Stripe_connect string
}

type MealRequest struct {
	Id       int64
	Guest_id int64
	Meal_id  int64
	Status   int64
}

func GetUserById(db *sql.DB, id int64) (*UserData, error) {
	row := db.QueryRow(`SELECT Id, Email, First_name, Last_name,
		Password_salt, Password_hash,
		Password_reset_key, Phone, Verified
        FROM User WHERE Id = ?`, id)
	return readUserLine(row)
}

func GetGuestByFbId(db *sql.DB, fb_id string) (*GuestData, error) {
	row := db.QueryRow(`SELECT Id, Email, Name,
		Facebook_id, Stripe_cust_id 
		FROM Guest WHERE Facebook_id = ?`, fb_id)
	return readGuestLine(row)
}

func GetGuestById(db *sql.DB, id int64) (*GuestData, error) {
	row := db.QueryRow(`SELECT Id, Email, Name, 
		Facebook_id, Stripe_cust_id 
		FROM Guest WHERE Id = ?`, id)
	return readGuestLine(row)
}

func GetFacebookPic(fb_id string) string {
	return "https://graph.facebook.com/" + fb_id + "/picture?width=400"
}

func GetHostByGuestId(db *sql.DB, id int64) (*HostData, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Address, Phone, 
		Stripe_connect 
		FROM Host WHERE GuestId = ?`, id)
	return readHostLine(row)
}

func GetHostById(db *sql.DB, id int64) (*HostData, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Address, Phone, 
		Stripe_user_id 
		FROM Host WHERE Id = ?`, id)
	return readHostLine(row)
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
	row := db.QueryRow(`SELECT Id, Host_id, Price, Title
        FROM Meal 
        WHERE Id = ?`, id)
	return readMealLine(row)
}

func GetMealRequestByGuestIdAndMealId(db *sql.DB, guest_id int64, meal_id int64) (meal_req *MealRequest, err error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Meal_id, Status
        FROM MealRequest
        WHERE Guest_id = ? AND Meal_id = ?`, guest_id, meal_id)
	meal_req, err = readMealRequestLine(row)
	// err thrown will be "NoRowsError"
	if err != nil {
		return nil, err
	}
	return meal_req, nil
}

func GetMealRequestById(db *sql.DB, request_id int64) (*MealRequest, error) {
	row := db.QueryRow(`SELECT Id, Guest_id, Meal_id, Status
        FROM MealRequest
        WHERE Id = ?`, request_id)
	meal_req, err := readMealRequestLine(row)
	if err != nil {
		return nil, err
	}
	return meal_req, nil
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

func UpdateGuestFbToken(db *sql.DB, fb_id int64, fb_token string) error {
	_, err := db.Exec(`
		UPDATE Guest
		SET Facebook_long_token = ?
		WHERE Facebook_id =?`,
		fb_token, fb_id,
	)
	return err
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
		&guest_data.Name,
		&guest_data.Facebook_id,
		&guest_data.Stripe_cust_id,
	); err != nil {
		return nil, err
	}
	return guest_data, nil
}

func readHostLine(row *sql.Row) (*HostData, error) {
	host_data := new(HostData)
	if err := row.Scan(
		&host_data.Id,
		&host_data.Guest_id,
		&host_data.Address,
		&host_data.Phone,
		&host_data.Stripe_connect,
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
		&meal_req.Status,
	); err != nil {
		return nil, err
	}
	return meal_req, nil
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

func SaveMealRequest(db *sql.DB, meal_req *MealRequest) error {
	_, err := db.Exec(
		`INSERT INTO MealRequest
		(Guest_id, Meal_id, Status)
		VALUES
		(?, ?, ?)
		`,
		meal_req.Guest_id,
		meal_req.Meal_id,
		0,
	)
	return err
}

func SaveStripeToken(db *sql.DB, stripe_token string, guest_data *GuestData) error {
	stripe.Key = "***REMOVED***"

	customerParams := &stripe.CustomerParams{
		Desc:  guest_data.Name,
		Email: guest_data.Email,
	}
	customerParams.SetSource(stripe_token) // obtained with Stripe.js
	c, err := customer.New(customerParams)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		UPDATE Guest
		SET Stripe_cust_id = ?
		WHERE Id = ?`,
		c.ID, guest_data.Id,
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
