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
