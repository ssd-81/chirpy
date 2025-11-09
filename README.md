# chirpy
go based http server without frameworks


## technologies used 
Go as the core programming langugae 
Postgresql as database
    - did not used ORM
    - used sqlc to generate sql mapped go code (kinda like ORM, but not exactly)


## core idea
This was built as a learning project following the go http servers course in the boot.dev curriculum. 
---
This project is an HTTP server focused on the following
- user creation
- posts
- updation / deletion posts
- authentication and authorization
- webhooks 


## endpoints 
API Endpoints

GET /api/healthz
Health check endpoint; returns "OK" if the server is running.

POST /api/users
Register a new user with an email and password.

POST /api/login
Authenticate a user and return access and refresh tokens.

POST /api/refresh
Exchange a valid refresh token for a new access token.

POST /api/revoke
Revoke a refresh token so it can no longer be used.

PUT /api/users
Update the authenticated user’s email and password.

POST /api/chirps
Create a new chirp (max 140 characters) for the authenticated user.

GET /api/chirps
Retrieve all chirps, optionally filtered by author_id and sorted by sort=asc.

GET /api/chirps/{chirpID}
Retrieve a single chirp by its ID.

DELETE /api/chirps/{chirpID}
Delete a chirp owned by the authenticated user.

Admin API

GET /admin/metrics
View total number of static file requests (admin dashboard).

POST /admin/reset
Reset metrics and delete all users (available only in dev mode).

Webhook API

POST /api/polka/webhooks
Handle Polka webhook events; upgrades users on user.upgraded event.