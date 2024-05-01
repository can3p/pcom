package util

import "os"

func SiteRoot() string {
	return os.Getenv("SITE_ROOT")
}
