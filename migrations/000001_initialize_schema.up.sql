CREATE TABLE "users" (
    "id" bigserial PRIMARY KEY,
    "name" varchar(255) NOT NULL,
    "email" varchar(255) UNIQUE NOT NULL,
    "password" char(60) NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);
