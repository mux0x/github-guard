package main

import (
	"bufio"
	"context"
	"fmt"
	"log"

	// "flag"
	// "fmt"
	"io/ioutil"
	"net/http"

	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gtuk/discordwebhook"
	jsoniter "github.com/json-iterator/go"
	flag "github.com/spf13/pflag"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client http.Client

var target string
var Target string
var DorkFile string
var Keyword string
var Token string
var TokenFile string
var ListDBBool bool
var ListTargetsBool bool
var NeedWait bool
var NeedWaitSecond int64
var EachWait int64
var Tokennum = 0
var Tokens []string
var Dorks []string
var TargetsFile string
var ErrorTimes = 0
var ErrorMaxTimes = 100
var collection *mongo.Collection
var collectionExists bool

// var dumpDataBase bool
var autoScan int
var discordWebhook string
var telegramBotToken string
var telegramChatID string

func Connect() (*mongo.Client, context.Context, error) {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")

	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to MongoDB: %w", err)
	}

	ctx := context.Background()

	return client, ctx, nil
}
func getCollectionNames() []string {
	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")

	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		fmt.Println("Error connecting to MongoDB:", err)
		return nil
	}

	ctx := context.Background()

	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("github_guard")

	collectionNames, err := db.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		log.Fatal(err)
	}
	return collectionNames
}
func query(dork string, token string) {

	client, ctx, err := Connect()

	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	db := client.Database("github_guard")

	collectionNames := getCollectionNames()
	if err != nil {
		log.Fatal(err)
	}
	collectionExists = false
	for _, name := range collectionNames {
		if Target != "" {
			if name == Target {
				collectionExists = true
				break
			}
		}

	}

	if !collectionExists {
		if Target != "" {
			err = db.CreateCollection(ctx, Target)
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	if Target != "" {
		collection = client.Database("github_guard").Collection(Target)
	}
	if err != nil {
		log.Fatal(err)
	}

	guri := "https://api.github.com/search/code"
	uri, _ := url.Parse(guri)

	param := url.Values{}
	param.Set("q", dork)
	uri.RawQuery = param.Encode()

	req, _ := http.NewRequest("GET", uri.String(), nil)
	req.Header.Set("accept", "application/vnd.github.v3+json")
	req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	req.Header.Set("User-Agent", "HelloGitHub")

	resp, err := Client.Do(req)

	if err != nil {
		color.Red("error: %v", err)
	} else {
		source, _ := ioutil.ReadAll(resp.Body)
		var tmpSource map[string]jsoniter.Any
		_ = jsoniter.Unmarshal(source, &tmpSource)

		if tmpSource["documentation_url"] != nil {
			ErrorTimes += 1
			if ErrorTimes >= ErrorMaxTimes {
				color.Red("Too many errors, auto stop")
				os.Exit(0)
			}
			if NeedWait {
				color.Blue("error: %s ; and we need wait %ds", jsoniter.Get(source, "documentation_url").ToString(), NeedWaitSecond)
				time.Sleep(time.Second * time.Duration(NeedWaitSecond))
				token = getToken()
				query(dork, token)
			} else {
				color.Red("error: %s", jsoniter.Get(source, "documentation_url").ToString())
			}
		} else if tmpSource["total_count"] != nil {
			totalCount := jsoniter.Get(source, "total_count").ToInt()
			totalCountString := color.YellowString(fmt.Sprintf("(%s)", strconv.Itoa(totalCount)))
			uriString := color.GreenString(strings.Replace(uri.String(), "https://api.github.com/search/code", "https://github.com/search", -1) + "&s=indexed&type=Code&o=desc")
			filter := bson.M{"dork": dork}

			var doc bson.M
			if err := collection.FindOne(ctx, filter).Decode(&doc); err == mongo.ErrNoDocuments {
				// If the document does not exist, create a new one
				doc := bson.M{"dork": dork, "count": totalCount, "uriofdork": strings.Replace(uri.String(), "https://api.github.com/search/code", "https://github.com/search", -1) + "&s=indexed&type=Code&o=desc"}
				_, err := collection.InsertOne(ctx, doc)
				if err != nil {
					log.Fatal(err)
				}
			} else if err != nil {
				log.Fatal(err)
			} else {
				count := int(doc["count"].(int32))

				if count < totalCount {
					update := bson.M{"$set": bson.M{"count": count}}
					_, err := collection.UpdateOne(ctx, filter, update)
					if err != nil {
						log.Fatal(err)
					}
					if discordWebhook != "" {
						url := discordWebhook
						content := "`New count found for dork: " + dork + ", it was " + strconv.Itoa(count) + " and now it is " + strconv.Itoa(totalCount) + "`"
						username := "github-guard"
						message := discordwebhook.Message{
							Username: &username,
							Content:  &content,
						}

						err := discordwebhook.SendMessage(url, message)
						if err != nil {
							log.Fatal(err)
						}
					} else if telegramBotToken != "" && telegramChatID != "" {
						apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", telegramBotToken)

						// Construct the form data for the request
						formData := url.Values{}
						formData.Set("chat_id", telegramChatID)
						formData.Set("text", "New count found for dork: "+dork)

						// Make a POST request to the sendMessage API endpoint with the form data
						response, err := http.PostForm(apiURL, formData)

						// Check for errors and print the response body
						if err != nil {
							fmt.Println(err)
							return
						}

						defer response.Body.Close()
					}

					fmt.Println("`added new count to dork: ", dork, " it was ", count, " and now it is ", totalCount, "`")
				}

			}

			fmt.Println(dork, " | ", totalCountString, " | ", uriString)
		} else {
			color.Blue("unknown error happened: %s", string(source))
		}
	}

}
func StringInSlice(s string, slice []string) bool {
	for _, value := range slice {
		if value == s {
			return true
		}
	}
	return false
}

func menu() {

	flag.StringVar(&DorkFile, "gd", "", "github dorks file path")
	flag.StringVar(&Keyword, "gk", "", "github search keyword")
	flag.StringVar(&Token, "token", "", "github personal access token")
	flag.StringVar(&TokenFile, "tf", "", "github personal access token file")
	flag.StringVar(&Target, "target", "t", "target which search in github")
	flag.StringVar(&TargetsFile, "tl", "", "list of targets to search for leaks")
	flag.BoolVar(&NeedWait, "nw", true, "if get github api rate limited, need wait ?")
	flag.Int64Var(&NeedWaitSecond, "nws", 10, "how many seconds does it wait each time")
	flag.Int64Var(&EachWait, "ew", 0, "how many seconds does each request should wait ?")
	flag.IntVar(&autoScan, "auto", 0, "scan a target every n hours [provide number of hours]")
	flag.StringVar(&discordWebhook, "webhook", "", "discord webhook url")
	flag.StringVar(&telegramBotToken, "telegram-token", "", "telegram bot token")
	flag.StringVar(&telegramChatID, "telegram-chat-id", "", "telegram chat id")
	flag.Usage = func() {
		color.Green("\n\t\t                                    /$$$$$$           ")
		color.Green("\t\t                                   /$$$_  $$          ")
		color.Green("\t\t /$$$$$$/$$$$  /$$   /$$ /$$   /$$| $$$$\\ $$ /$$   /$$")
		color.Green("\t\t| $$_  $$_  $$| $$  | $$|  $$ /$$/| $$ $$ $$|  $$ /$$/")
		color.Green("\t\t| $$ \\ $$ \\ $$| $$  | $$ \\  $$$$/ | $$\\ $$$$ \\  $$$$/ ")
		color.Green("\t\t| $$ | $$ | $$| $$  | $$  >$$  $$ | $$ \\ $$$  >$$  $$ ")
		color.Green("\t\t| $$ | $$ | $$|  $$$$$$/ /$$/\\  $$|  $$$$$$/ /$$/\\  $$")
		color.Green("\t\t|__/ |__/ |__/ \\______/ |__/  \\__/ \\______/ |__/  \\__/")
		fmt.Println()
		color.Green("\t\t[+] github-guard")
		color.Green("\t\t[+] github@mux0x")
		fmt.Println()
		fmt.Println("Usage of github-guard:")
		fmt.Println("\tModes of using github-guard:")
		fmt.Println("\t  1. h4ck - searches for leaks in github")
		fmt.Println("\t  2. dump - dumps all dorks results for a specific org / target - requires target")
		fmt.Println("\t  3. list - lists all targets in the database")
		// fmt.Println()
		flag.PrintDefaults()
	}
}

func parseparam() {
	if Token != "" {
		Tokens = []string{Token}
	} else if TokenFile != "" {
		tfres, err := ioutil.ReadFile(TokenFile)
		if err != nil {
			color.Red("file error: %v", err)
			os.Exit(0)
		} else {
			tfresLine := strings.Split(string(tfres), "\n")
			for {
				if tfresLine[len(tfresLine)-1] == "" {
					tfresLine = tfresLine[:len(tfresLine)-1]
				} else {
					break
				}
			}
			Tokens = tfresLine
		}
	}
	if Keyword != "" {
		Dorks = []string{Keyword}
	} else if DorkFile != "" {
		dkres, err := ioutil.ReadFile(DorkFile)
		if err != nil {
			color.Red("file error: %v", err)
			os.Exit(0)
		} else {
			dkresLine := strings.Split(string(dkres), "\n")
			for {
				if dkresLine[len(dkresLine)-1] == "" {
					dkresLine = dkresLine[:len(dkresLine)-1]
				} else {
					break
				}
			}
			Dorks = dkresLine
		}
	}
	if flag.Args()[0] == "h4ck" {
		color.Blue("[+] got %d tokens and %d dorks\n\n", len(Tokens), len(Dorks))
	}
}

func getToken() string {
	token := Tokens[Tokennum]
	Tokennum += 1
	if len(Tokens) == Tokennum {
		Tokennum = 0
	}
	return token
}

func readFileLines(filepath string) ([]string, error) {
	// Open the file
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create an empty slice to hold the lines
	lines := []string{}

	// Use a bufio.Scanner to read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Append each line to the slice
		lines = append(lines, scanner.Text())
	}

	// Handle any errors that may have occurred during scanning
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func main() {
	modes := []string{"h4ck", "dump", "list"}
	menu()
	flag.Parse()
	parseparam()
	Client = http.Client{}
	args := flag.Args()

	if flag.NFlag() == 0 && args[0] != "list" || !StringInSlice(flag.Args()[0], modes) {
		if !StringInSlice(flag.Args()[0], modes) {
			color.Red("Unrecognized mode")
		}
		flag.Usage()
		os.Exit(0)
	}

	if args[0] == "list" {

		collectionNames := getCollectionNames()

		maxLength := 0
		for _, name := range collectionNames {
			if len(name) > maxLength {
				maxLength = len(name)
			}
		}

		dbStructure := "+----------------------------------+\n"
		dbStructure += fmt.Sprintf("|           %s           |\n", "Github guard")
		dbStructure += "+----------------------------------+\n"
		for _, coll := range collectionNames {
			dbStructure += fmt.Sprintf("|  %-30s  |\n", coll)
		}
		dbStructure += "+----------------------------------+"

		fmt.Println(dbStructure)

	} else if args[0] == "dump" {
		client, _, err := Connect()
		if Target == "" {
			color.Red("require target / org")
			os.Exit(0)
		} else {
			target = Target
		}
		if err != nil {
			fmt.Println("Error connecting to MongoDB:", err)
			return
		}

		for i, name := range getCollectionNames() {
			if target != name {
				if i == len(getCollectionNames())-1 {
					fmt.Println("The provided target is not in the database")
					os.Exit(0)
				}
			} else {

				collection := client.Database("github_guard").Collection(name)
				cur, err := collection.Find(context.Background(), bson.M{})
				if err != nil {
					log.Fatal(err)
				}
				defer cur.Close(context.Background())
				fmt.Printf("|%s|%s|\n", strings.Repeat("-", 32), strings.Repeat("-", 8))
				mx := fmt.Sprintf("| %-30s | %6s |", "dork", "count")
				fmt.Println(mx)
				fmt.Printf("|%s|%s|\n", strings.Repeat("-", 32), strings.Repeat("-", 8))
				for cur.Next(context.Background()) {
					var result bson.M
					err := cur.Decode(&result)
					if err != nil {
						log.Fatal(err)
					}

					dork := result["dork"].(string)
					count := result["count"].(int32)
					row := fmt.Sprintf("| %-30s | %6d |", dork, count)
					fmt.Println(row)

				}
				fmt.Printf("|%s|%s|\n", strings.Repeat("-", 32), strings.Repeat("-", 8))
				if err := cur.Err(); err != nil {
					log.Fatal(err)
				}

				// Disconnect the client
				err = client.Disconnect(context.Background())
				if err != nil {
					log.Fatal(err)
				}
				break
			}
		}

	} else if args[0] == "h4ck" {
		if Token == "" && TokenFile == "" {
			color.Red("require token or tokenfile")
			os.Exit(0)
		}
		if DorkFile == "" && Keyword == "" {
			color.Red("require keyword or dorkfile")
			os.Exit(0)
		}

		for {
			for _, dork := range Dorks {
				token := getToken()
				var queryStr string
				switch {
				case Target != "":
					queryStr = fmt.Sprintf("%s %s", Target, dork)
				case TargetsFile != "":
					lines, err := readFileLines(TargetsFile)
					if err != nil {
						fmt.Println(err)
						os.Exit(1)
					}
					for _, line := range lines {
						queryStr = fmt.Sprintf("%s %s", line, dork)
						query(queryStr, token)
						time.Sleep(time.Second * time.Duration(EachWait))
					}
					continue // skip query() call below

				}
				query(queryStr, token)
				time.Sleep(time.Second * time.Duration(EachWait))
			}
			if autoScan > 0 {
				time.Sleep(time.Hour * time.Duration(autoScan))
				fmt.Println("Waiting for next cycle, its after " + strconv.Itoa(autoScan) + " hours")
			} else {
				break
			}
		}

	}
}
