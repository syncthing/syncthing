# Tests

## Running Tests

`go test` is used for testing. A running PostgreSQL
server is required, with the ability to log in. The
database to connect to test with is "pqgotest," on
"localhost" but these can be overridden using [environment
variables](https://www.postgresql.org/docs/9.3/static/libpq-envars.html).

Example:

	PGHOST=/run/postgresql go test

## Benchmarks

A benchmark suite can be run as part of the tests:

	go test -bench .

## Example setup (Docker)

Run a postgres container:

```
docker run --expose 5432:5432 postgres
```

Run tests:

```
PGHOST=localhost PGPORT=5432 PGUSER=postgres PGSSLMODE=disable PGDATABASE=postgres go test
```
