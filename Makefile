all:
	go get "code.google.com/p/gcfg"
	go get "github.com/go-sql-driver/mysql"
	go get "code.google.com/p/go-uuid/uuid"
	go get "code.google.com/p/go.crypto/pbkdf2"
	go get "github.com/rschlaikjer/go-apache-logformat"
	go get "bitbucket.org/ckvist/twilio/twirest"
	go get "github.com/stripe/stripe-go"
	go get github.com/sendgrid/sendgrid-go
	go build -o cornerd -ldflags "-X main.build_version ${VERSION}${VERSION_DIRTY}"