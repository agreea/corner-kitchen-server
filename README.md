# corner-kitchen-server

## MVP Stories
* As a user, I want to be able to see what food trucks are around me 
* As a user, I want to be able to see what items are on each food truck's menu, including a title and price
* As a user, I want to be able to select a pickup time for my order
* As a user, I want to know that my order has been sent to the food truck
* As a user, I want to see a history of my orders
* As a user, I want to be able to be able to pay for my order from the app through Stripe
* As a foodtruck, I want to broadcast my location, menu, and closing time to nearby users
* As a foodtruck, I want to receive orders over SMS
* As a foodtruck, I want to be able to receive payment for food purchased on the app through Stripe

## MVP Models

### Users
* id
* name
* Credit card info? Idk how Stripe integration works, but that seems like the easiest way to go
* Order history

### Foodtrucks
* id
* name
* location
* menu_id (could be same as id?)
* open_til (epoch seconds, 0 == closed)
* phone_number (we'll send them orders over Twilio)
* profile_picture (String, url)
* some payout information? Yet again, not sure how Stripe does this.

### Menu Items
* id
* title
* price
* description
* foodtruck_id

### Orders
* foodtruck_id
* user_id
* timestamp (epoch seconds)
* menu_item_id
* customer_location (at time of order)
* foodtruck_location (at time of order)
* pickup_time (15 minute intervals)

## Information Flow

### GET Feed
* User sends GET request to server with their location, id, and the desired radius (in meters)
* Server sends back the within-radius foodtrucks' name, id, location, open_til, and profile_picture_url

### GET Menu
* User sends GET request for the menu items associated with foodtruck_id
* Server sends back each menu item's title, price, and description

### POST Order
* User sends an order consisting of user_id, Array of menu_item_ids, pickup_time, customer_location, and foodtruck_id to server, timestamp
* Server sends confirmation if foodtruck is still open and the order was sent, an error with a message otherwise

### GET History
* User sends id
* Server sends all orders associated with that name
