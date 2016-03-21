package main

import (
	"code.google.com/p/gcfg"
	"log"
	"strings"
)

var build_version string

type Config struct {
	Mysql struct {
		MysqlUser       string
		MysqlPassword   string
		MysqlIPProto    string
		MysqlServerAddr string
		MysqlServerPort string
		MysqlDatabase   string
	}

	Logging struct {
		LogFile       string
		AccessLogFile string
	}

	Network struct {
		BindAddress  string
		BindPort     string
		ReadTimeout  string
		WriteTimeout string
	}

	Arguments struct {
		LogToStderr bool
	}

	Memcache struct {
		Host string
	}

	Twilio struct {
		SID   string
		Token string
		From  string
	}
	
	SendGrid struct {
		User string
		Pass string
	}

	HTML struct {
		Path string
	}
	
	Stripe struct {
		Key string
	}

	Mailchimp struct {
		User string
		Pass string
	}
	Facebook struct {
		Key string
	}
	Image struct {
		Path string
	}

	Version struct {
		V string
	}
}
// MAKE SURE TO CHANGE THIS WHEN MIGRATING TO POSTGRES
func (kc Config) GetSqlURI() string {
	mysql_auth_strings := []string{kc.Mysql.MysqlUser,
		":",
		kc.Mysql.MysqlPassword,
		"@",
		kc.Mysql.MysqlIPProto,
		"(",
		kc.Mysql.MysqlServerAddr,
		":",
		kc.Mysql.MysqlServerPort,
		")/",
		kc.Mysql.MysqlDatabase,
		"?parseTime=true",
	}
	return strings.Join(mysql_auth_strings, "")
}

func LoadConfiguration(config_path string) Config {
	kc := Config{}
	err := gcfg.ReadFileInto(&kc, config_path)
	if err != nil {
		log.Fatal("Failed to parse gcfg data: ", err)
	}
	return kc
}
