package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"

	itempb "item-management/item"

	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	status "google.golang.org/grpc/status"
)

var (
	myServer server
)

const (
	CERT_FILE = "../ssl/server.crt"
	KEY_FILE  = "../ssl/server.pem"
)

// Implement CartManagementServer interface
type server struct {
	itempb.UnimplementedCartManagementServer

	items []*itempb.Item
	cart  map[string][]*itempb.Item
}

func (s *server) Retrieve(ctx context.Context, i *itempb.Item) (*itempb.Item, error) {
	for _, value := range s.items {
		if i.Id == value.Id { // nil ptr deref
			return value, nil
		}
	}

	// NO item found
	return nil, errors.New("Item not found")
}

// need an item under category "Peripherals"
func (s *server) List(i *itempb.Item, stream itempb.CartManagement_ListServer) error {
	if len(s.items) < 1 {
		return errors.New("emptylist")
	}
	for _, value := range s.items {
		// Update: only send if item category matches
		if err := stream.Send(value); err != nil {
			return err
		}
	}

	return nil
}

func (s *server) Add(stream itempb.CartManagement_AddServer) error {
	defer stream.SendAndClose(&itempb.ReportSummary{
		Id:      "444",
		Message: "Items successfully added",
	})
	for {
		item, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			status.Errorf(status.Code(err), "Error reading stream from client: %v\n", err)

			return err
		}

		s.items = append(s.items, item)
	}
	return nil
}

// Bidi-streaming
func (s *server) GetCart(stream itempb.CartManagement_GetCartServer) error {
	for {
		user, err := stream.Recv()
		if err == io.EOF {
			log.Printf("End of line: %v\n", err)
			break
		}
		if err != nil {
			// status.Errorf(status.Code(err), "Error reading stream from client: %v\n", err)
			status.Errorf(codes.Unknown, "Error reading stream from client: %v\n", err)
			return err
		}

		log.Printf("[Server] Processing user: %v\n", user.Name)

		// retrieve cart
		if userCart, exist := s.cart[user.Name]; exist {
			for _, item := range userCart {
				if err := stream.Send(item); err != nil {

					return err
				}
			}
		} else {
			// time.Sleep(time.Millisecond * 200)
			if err := stream.Send(&itempb.Item{Name: "User does not exist"}); err != nil {
				return err
			}
		}
	}
	log.Println("[Server] Done processing all user")
	return nil
}

func (s *server) Delete(ctx context.Context, i *itempb.Item) (*itempb.ReportSummary, error) {
	if len(s.items) < 1 {
		return nil, errors.New("cart empty")
	}
	// search for item in list by id
	for index, item := range s.items {
		// append(s[:index], s[index+1:]...)
		if item.Id == i.Id {
			// s.items[index] = nil
			s.items = append(s.items[:index], s.items[index+1:]...)
			return &itempb.ReportSummary{Message: "Item Id: " + i.Id + " successfully removed. Current items: " + fmt.Sprint(len(s.items))}, nil
		}
	}
	return nil, errors.New("Item not found")
}

// load items from a JSON file.
func (s *server) loadFeatures(filePath string) {
	var data []byte

	var err error
	data, err = ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to load default features: %v", err)
	}

	if err := json.Unmarshal(data, &s.items); err != nil {
		log.Fatalf("Failed to load default features: %v", err)
	}
}

func init() {
	// create server
	myServer = server{}
	myServer.loadFeatures("item.json")

}

func main() {

	// Establish a TCP conn
	conn, err := net.Listen("tcp", ":9000")

	if err != nil {
		log.Fatalf("Error establishing connection to server: %v\n", err)
	}

	tls := false
	var opts []grpc.ServerOption
	if tls {
		creds, err := credentials.NewServerTLSFromFile(CERT_FILE, KEY_FILE)
		if err != nil {
			log.Fatalf("error loading cert: %v", err)
		}

		opts = append(opts, grpc.Creds(creds))
	}

	// "bind" service to grpc server
	grpcServer := grpc.NewServer(opts...)
	// grpcServer := grpc.NewServer()
	itempb.RegisterCartManagementServer(grpcServer, &myServer)

	interrupt := make(chan os.Signal)
	go func() {
		log.Println("[Server] starting server...")
		// serve
		if err := grpcServer.Serve(conn); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// fill interrupt <- ? when user Ctrl-C
	signal.Notify(interrupt, os.Interrupt)

	// block
	<-interrupt
	log.Print("Interrupt signal detected!................")

	// Close server
	log.Println("Stopping Server")
	grpcServer.Stop()

	// Close listener
	log.Println("Closing listener...")
	conn.Close()
}
