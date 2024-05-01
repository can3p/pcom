package util

import "os"

func InCluster() bool {
	_, ok := os.LookupEnv("FLY_APP_NAME")

	return ok
}
