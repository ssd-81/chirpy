-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(), 
    NOW(),
    NOW(),
    $1,
    $2
)
RETURNING *;


-- name: DeleteUsers :exec
DELETE FROM users;


-- name: GetSpecificUser :one
SELECT * FROM users
WHERE email = $1;

-- name: UpdateEmailAndPassword :exec
UPDATE users SET 
updated_at = NOW(),
email = $1,
hashed_password = $2
WHERE id = $3;

-- name: GetSpecificUserWithoutPassword :one
SELECT id, created_at, updated_at, email FROM users
WHERE id = $1
LIMIT 1;