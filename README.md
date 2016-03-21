# corner-kitchen-server

## Server-Servlet Design
The server breaks down its functions into servlets, each of which focuses on a specific data model.
The client accesses these servlets through api endpoints: "api/meal/" and then include the method and associated data in the body. Through reflection, the server routes the request to the appropriate servlet function.

## Goroutines
This server utilizes multithreading to manage recurring routines, such as processing payments, clearing expired session tokens from the database, and nudging users to leave reviews after meals occur.

## Interesting Problems
Read about how we built a mobile-friendly address fuzzing system (of the kind you see on Airbnb listings) here.
