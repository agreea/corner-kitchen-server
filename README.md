# corner-kitchen-server

## Structure
The server handles HTTP requests by routing them to the appropriate servlet, which focuses on a specific set of tasks related to its associated data model. At runtime these servlets and their methods are loaded into a map. API requests target endpoints for a specific servlet, eg "api/meal/", and then include the method and associated data in the body. If the servlet exists in the map, it then checks to see if the associated method exists. If the method does not exist on that servlet, the client receives an error. If the method does exist, it is called and returns either an APISuccess with return data or an APIError with an error message.

Database interactions are handled in schema.go, while user session management is handled in session.go.
### Goroutines
This server utilizes multithreading to manage recurring routines, such as processing payments, clearing expired session tokens from the database, and nudging users to leave reviews after meals occur.

## Usage 
Use either make or go get . && go build to fetch dependencies and build the binary.

The binary must have a server.gcfg in the same directory from which it reads environment-specific variables such as database credientials and API keys.


## Interesting Problems
Read about how we built a mobile-friendly address fuzzing system (of the kind you see on Airbnb listings) here.
