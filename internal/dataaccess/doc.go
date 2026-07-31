// Package dataaccess defines provider-independent data access ports.
// The current application keeps using internal/database as its composition
// root until each business workflow has a contract-tested adapter.
//
// Implementations may use SQLite, Cloudflare D1, AWS RDS for PostgreSQL, a
// local filesystem, R2, or S3. Provider-specific handles, locators, URLs,
// clients, and error types must not cross these interfaces.
package dataaccess
