### For Installation Guide refer:
- https://github.com/golang-migrate/migrate/blob/master/cmd/migrate/README.md

### For best practices refer: 
- https://github.com/golang-migrate/migrate/blob/master/MIGRATIONS.md

### Facing a dirty migration issue? Refer: 
- https://github.com/golang-migrate/migrate/blob/master/FAQ.md#what-does-dirty-database-mean

### How are migrations applied? 

- Whenever you need to make changes to the current applied schema, in the migrations folder, run `migrate create -ext sql -seq {{file_name}}`
- A new version of a migration will be created. Once the changes are written and verified.
- Make sure to write a rollback script in `__.down.sql` file. Otherwise the rollback of migrations won't be possible.
- To apply the latest version of the migration, simply execute `go run migrate_db.go`
- For additional utilities, for eg: drop the entire db, simply pass drop as a runtime argument like `go run migrate_db.go drop`
- **DO NOT** drop `schema_migrations` table as that will lead to migration failure.
