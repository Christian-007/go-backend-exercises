include .env

DIR ?= ./cmd/main.go

run:
	sh -c 'env $$(cat .env | xargs) go run ${DIR}'

migrate_up_all:
	migrate -path migrations -database "${POSTGRES_URL}" -verbose up

migrate_down_all:
	migrate -path migrations -database "${POSTGRES_URL}" -verbose down

migrate_up_1:
	migrate -path migrations -database "${POSTGRES_URL}" -verbose up 1

migrate_down_1:
	migrate -path migrations -database "${POSTGRES_URL}" -verbose down 1

.PHONY: run migrate_up_all migrate_down_all migrate_up_1 migrate_down_1