PWD = $(shell pwd)

migration-script:
	cd migrations && migrate create -ext sql -seq $(filename)

make migrate:
	go run migrate_db.go