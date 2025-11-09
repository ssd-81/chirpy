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


## usage 

- create a .env at the root of the project ; make changes accordingly
```
DB_URL="postgres://your_db_name:password@localhost:5432/chirpy?sslmode=disable"
```
- 