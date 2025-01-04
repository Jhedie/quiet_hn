package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Jhedie/quiet_hn/hn"
)

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var client hn.Client
		ids, err := client.TopItems()
		if err != nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}
		var stories []item

		//safely append results to slice/stories
		var mu sync.Mutex

		//Wait group to wait for all go routines to finish
		var wg sync.WaitGroup

		storyChan := make(chan item, numStories)
		done := make(chan struct{})

		for _, id := range ids {
			wg.Add(1)

			go func(id int) {
				defer wg.Done()
				select {
				case <-done:
					// Exit early if enough stories have been collected
					return
				default:

					hnItem, err := client.GetItem(id)
					if err != nil {
						fmt.Printf("Error fetching data for ID %d: %v\n", id, err)
					}
					item := parseHNItem(hnItem)
					if isStoryLink(item) {
						select {
						case storyChan <- item:
						case <-done:
							// Exit if enough stories are already collected
							return
						}
					}
				}
			}(id)
		}
		// Close the channel once all goroutines finish
		go func() {
			wg.Wait()
			close(storyChan)
		}()

		// Collect stories from the channel
		for story := range storyChan {
			mu.Lock()
			stories = append(stories, story)
			if len(stories) >= numStories {
				// Signal other goroutines to stop
				close(done)
				mu.Unlock()
				break
			}
			mu.Unlock()
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
