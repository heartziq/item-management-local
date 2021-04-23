package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	itempb "item-management/item"
	"log"
	"os"
	"time"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const (
	CERT_FILE = "../ssl/ca.crt" // Certificate Authority Trust certificate
)

func main() {
	// log.Fatalln(os.Args[1])
	// dial
	// conn, err := grpc.Dial(":9000", grpc.WithInsecure())

	tls := false
	var conn *grpc.ClientConn
	var err error
	if tls {
		creds, sslErr := credentials.NewClientTLSFromFile(CERT_FILE, "")
		if sslErr != nil {
			log.Fatalf("ERror loading cert: %v", sslErr)
		}

		opts := grpc.WithTransportCredentials(creds)
		conn, err = grpc.Dial(":9000", opts)
	} else {
		conn, err = grpc.Dial(":9000", grpc.WithInsecure())
	}

	if err != nil {
		log.Fatalf("Error connecting to server: %v\n", err.Error())

	}

	defer conn.Close()

	// instantiate a client instance
	c := itempb.NewCartManagementClient(conn)

	// Create context obj - set deadline for every single RPC call
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// call function (Unary)
	// get Id
	var targetId string
	if len(os.Args) > 1 {
		targetId = os.Args[1]
	}

	response, err := c.Retrieve(ctx, &itempb.Item{Id: targetId})
	if err != nil {

		statusErr, ok := status.FromError(err)
		if ok {
			if statusErr.Code() == codes.DeadlineExceeded {
				log.Println("Deadline exceeded")

			} else {
				log.Printf("gRPC related error: %v\n", err)

			}
		} else {
			log.Fatalf("Unexpected error: %v\n", err)
		}
		// return (response will be nil) - null pointer dereference
	}

	log.Println(response)

	// Client-streaming method

	// load items to be added
	var items []*itempb.Item
	loadFeatures("item.json", &items)

	streamAdd, err := c.Add(ctx)

	for _, item := range items {
		if err := streamAdd.Send(item); err != nil {

			if respErr, ok := status.FromError(err); ok {
				// grpc error
				if ok {
					log.Printf("Error occur while adding item: %v\t%v\n", item.Id, respErr.Message())
					if respErr.Code() == codes.Unknown {
						log.Println("Unknown code")
					}
					break
				} else {
					// big grpc error
					log.Fatalf("Big error: %v", err)

				}
			}

		}
	}

	rpSummary, err := streamAdd.CloseAndRecv()
	if err != nil {
		log.Printf("Error occur while reading reportSummary: %v\n", err)
	}
	log.Println(rpSummary.GetMessage())

	usernames := []string{"hadziq", "myra", "syira"}

	// Bidi client
	streamGetCart, err := c.GetCart(ctx)
	chnl := make(chan int)

	go func() {
		for {
			item, err := streamGetCart.Recv()
			if err == io.EOF {
				log.Println("[Client] io.EOF: server done ending. closing channel....")
				close(chnl)
				return
			}
			if err != nil {
				return
			}
			time.Sleep(time.Millisecond * 400) //Simulate interleaving
			log.Println(item.GetName())
		}

	}()

	for _, username := range usernames {
		if err := streamGetCart.Send(&itempb.User{Name: username}); err != nil {
			log.Printf("Error occur while querying user: %v\t%v\n", username, err)
			return
		}
	}
	streamGetCart.CloseSend()

	// delete item
	streamDelete, err := c.Delete(ctx, &itempb.Item{Id: "3"})
	if err != nil {
		log.Println(err)
	}

	log.Println(streamDelete.GetMessage())

	// call function (Server-streaming)
	emptyItemPtr := new(itempb.Item)

	stream, err := c.List(ctx, emptyItemPtr)
	listChnl := make(chan struct{})
	if err != nil {
		log.Fatalf("Error getting response back from server: %v\n", err)
	}
	// concurrency can be applied here
	go func() {
		for {
			item, err := stream.Recv()
			if err == io.EOF {
				stream.CloseSend()
				close(listChnl)
				break
			}
			if err != nil {
				log.Printf("Error occur while reading *itempb.Item: %v\n", err)
				break
			}
			time.Sleep(time.Millisecond * 200)
			log.Println(item)
		}
	}()

	// for {
	// 	select {
	// 	case _, ok := <-chnl:
	// 		if !ok {
	// 			// close(chnl)

	// 		}

	// 	case _, ok := <-listChnl:
	// 		if !ok {
	// 			// close(listChnl)
	// 		}

	// 	}
	// }

	<-chnl
	<-listChnl

}

// load items from a JSON file.
func loadFeatures(filePath string, itemsToAdd *[]*itempb.Item) {
	var data []byte
	var err error
	data, err = ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to load default features: %v", err)
	}

	if err := json.Unmarshal(data, itemsToAdd); err != nil {
		log.Fatalf("Failed to load default features: %v", err)
	}
}
