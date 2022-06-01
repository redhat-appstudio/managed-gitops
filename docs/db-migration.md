# Managed GitOps Database Migration

[![GoDoc](https://godoc.org/github.com/redhat-appstudio/managed-gitops/migration?status.svg)](https://pkg.go.dev/mod/github.com/redhat-appstudio/managed-gitops/migration)
[![License](https://img.shields.io/:license-apache-blue.svg)](http://www.apache.org/licenses/LICENSE-2.0.html)

----

The Managed GitOps Database Migration component is responsible for making sure that the database can be versioned UP and DOWN according to the need.

## Getting started with database migrations for manged-gitops

### For the Installation Guide refer to the [migration guide](https://github.com/golang-migrate/migrate/blob/master/cmd/migrate/README.md).

### For best practices refer to the [migrate pkg documentations](https://github.com/golang-migrate/migrate/blob/master/MIGRATIONS.md).

### Facing a dirty migration issue? Refer: [migrate FAQs](https://github.com/golang-migrate/migrate/blob/master/FAQ.md#what-does-dirty-database-mean)

## How are migrations applied? 

- Whenever you need to make changes to the currently applied schema, in the migrations folder, run `make migration-script`
- A new version of migration will be created. Once the changes are written and verified.
- Make sure to write a rollback script in the `__.down.sql` file. Otherwise, the rollback of migrations won't be possible.
- To apply the latest version of the migration, in the root managed-gitops directory, simply execute `make db-migrate`
- For additional utilities, for eg: drop the entire db, simply pass drop as a runtime argument like `make db-drop`
- **DO NOT** drop the `schema_migrations` table as that will lead to migration failure.

