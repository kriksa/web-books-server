package httpapi

import lru "github.com/hashicorp/golang-lru/v2"

var (
	missingCoverMarker = []byte{}
	imageCache         *lru.Cache[string, []byte]
)

func init() {
	var err error
	imageCache, err = lru.New[string, []byte](2000)
	if err != nil {
		panic(err)
	}
}
