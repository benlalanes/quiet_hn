package main

import (
	"encoding/json"
	//"encoding/json"
	"flag"

	//"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gophercises/quiet_hn/hn"
	"github.com/go-redis/redis"
)

type story struct {
	id int
	idx int
	content item
}

type StorySlice []story

func (s StorySlice) Less(i, j int) bool {
	return s[i].idx < s[j].idx
}

func (s StorySlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s StorySlice) Len() int {
	return len(s)
}

var client *redis.Client

const QuietHnResultsKey = "quiet-hn-results:"

type Info struct {
	Id int
	Name string
}

func main() {
	// parse flags

	client = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		Password: "",
		DB: 0,
	})

	//resultsStr, err := client.Get("quiet-hn-results:").Result()
	client.Del(QuietHnResultsKey).Result()
	//if err == redis.Nil {
	//	fmt.Println("No cache found.")
	//}
	//fmt.Printf("%q\n", resultsStr)
	//fmt.Println(err)
	//
	//
	//infos := []*Info{
	//	{Id: 0, Name: "Benjamin"},
	//}

	//info := Info{
	//	Id: 0,
	//	Name: "Michele",
	//}

	//infosStr, err := json.Marshal(infos)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//fmt.Println(string(infosStr))
	//
	//var infos2 []Info
	//
	//err = json.Unmarshal(infosStr, &infos2)
	//if err != nil {
	//	log.Fatal(err)
	//}

	//fmt.Printf("%#v\n", infos2)


	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl, client))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", port), nil))
}

func handler(numStories int, tpl *template.Template, redisClient *redis.Client) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var client hn.Client
		ids, err := client.TopItems()
		if err != nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}

		var stories []item

		storiesJson, err := redisClient.Get(QuietHnResultsKey).Result()
		if err == redis.Nil {
			limit := int(1.25 * float64(numStories))

			ch := make(chan story, len(ids))

			for i := 0; i < limit; i++ {

				go func(id int, idx int) {
					hnItem, err := client.GetItem(id)
					if err != nil {
						return
					}
					item := parseHNItem(hnItem)
					if isStoryLink(item) {

						ch <- story{
							id: id,
							idx: idx,
							content: item,
						}
					}
				}(ids[i], i)

			}

			var storiesIndexed StorySlice

			for i := 0; i < numStories; i++ {
				storiesIndexed = append(storiesIndexed, <-ch)
			}

			sort.Sort(storiesIndexed)

			for _, s := range storiesIndexed {
				stories = append(stories, s.content)
			}

			log.Printf("Trying to cache stories...\n")

			storiesJson, err := json.Marshal(stories)
			if err != nil {
				log.Println("Failed to cache top stories:", err)
				http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
				return
			}
			_, err = redisClient.Set(QuietHnResultsKey, string(storiesJson), 0).Result()
			if err != nil {
				log.Println("Failed to cache top stories:", err)
				http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
				return
			}

			log.Printf("Stories cached successfully.")
		} else {
			log.Printf("Using cache.\n")
			if err = json.Unmarshal([]byte(storiesJson), &stories); err != nil {
				log.Println("Failed to retrieve top stories from cache:", err)
				http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
				return
			}
		}

		data := templateData{
			Stories: stories,
			Time:    time.Now().Sub(start),
		}
		err = tpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Failed to process the template", http.StatusInternalServerError)
			return
		}
	})
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
