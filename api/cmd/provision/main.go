// provision is the admin CLI for creating organizations and users — there is
// deliberately no self-serve signup in this MVP.
//
// Usage:
//   go run ./cmd/provision -org "Acme Property Mgmt" -email pm@acme.test -password secret [-role admin] [-twilio +15551234567]
//   go run ./cmd/provision -add-user -org-id 1 -email second@acme.test -password secret
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/bcrypt"

	"maintenancehub/config"
	"maintenancehub/db"
)

func main() {
	var (
		orgName  = flag.String("org", "", "organization name to create")
		orgID    = flag.Int64("org-id", 0, "existing org id (with -add-user)")
		addUser  = flag.Bool("add-user", false, "add a user to an existing org")
		email    = flag.String("email", "", "user email")
		password = flag.String("password", "", "user password")
		name     = flag.String("name", "", "user display name")
		role     = flag.String("role", "admin", "user role: admin | pm | field")
		twilio   = flag.String("twilio", "", "org's Twilio phone number (E.164)")
		emailFrom = flag.String("email-from", "", "org's outreach from-address")
	)
	flag.Parse()

	config.LoadDotEnv(".env", "../.env")
	ctx := context.Background()
	pool, err := db.Connect(ctx, config.MustGet("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	if *email == "" || *password == "" {
		log.Fatal("-email and -password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}

	targetOrg := *orgID
	if !*addUser {
		if *orgName == "" {
			log.Fatal("-org is required when creating a new organization")
		}
		err = pool.QueryRow(ctx, `
			INSERT INTO organizations (name, twilio_phone_number, outreach_email_from)
			VALUES ($1, NULLIF($2,''), NULLIF($3,''))
			RETURNING id`, *orgName, *twilio, *emailFrom).Scan(&targetOrg)
		if err != nil {
			log.Fatalf("create org: %v", err)
		}
		fmt.Printf("created organization %q with id %d\n", *orgName, targetOrg)
	}
	if targetOrg == 0 {
		log.Fatal("-org-id is required with -add-user")
	}

	var userID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO org_users (org_id, email, password_hash, name, role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, targetOrg, *email, string(hash), *name, *role).Scan(&userID)
	if err != nil {
		log.Fatalf("create user: %v", err)
	}
	fmt.Printf("created user %s (id %d, role %s) in org %d\n", *email, userID, *role, targetOrg)
}
