package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/AlisterIgnatius/phoenix/common"
	"github.com/AlisterIgnatius/phoenix/rosette"
	"github.com/AlisterIgnatius/phoenix/storage"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/elastic/go-elasticsearch/v7"
)

var (
	content          storage.Repository
	log              *common.Logger
	topicSearch      storage.TopicSearch
	recommendService rosette.Rosette

	// These values are passed in at build-time w/ -ldflags (see: Makefile)
	awsAccount                string
	awsRegion                 string
	s3StructuredContentBucket string
	esEndpoint                string
	esIndex                   string
	esUsername                string
	esPassword                string
	rosetteAPIKey             string
)

func handleRequest(ctx context.Context, event events.SNSEvent) {
	for _, record := range event.Records {
		var err error
		var msg = &common.NodeStoredEvent{}
		var node *common.Section
		var topics []common.RelatedTopic

		// Deserialize message
		if err = json.Unmarshal([]byte(record.SNS.Message), msg); err != nil {
			log.Error("Unable to deserialize message payload: %s", err)
			continue
		}

		log.Debug("Processing Node published event: %+v", msg)

		// Retrieve Node object from storage
		if node, err = content.GetNode(msg.ID); err != nil {
			log.Error("Failed to retreive S3 object for node (ID=%s): %s", msg.ID, err)
			continue
		}

		log.Debug("Processing Node.Unsafe='%.24s...'", node.Text)

		// Fetch related-topics
		if topics, err = recommendService.Topics(node); err != nil {
			log.Error("Unable to retrieve related topics for %s: %s", msg.ID, err)
			continue
		}

		// Store related topics...
		if err = content.PutTopics(node, topics); err != nil {
			log.Error("Failed to store related-topics: %s", err)
		} else {
			// ...and then update topic index (if storage is successful)
			var stats *storage.UpdateStats
			if stats, err = topicSearch.Update(node, topics); err != nil {
				log.Error("Failed to index related-topics: %s", err)
			} else {
				log.Debug("Indexing stats: +%v", stats)
			}
		}
	}
}

func init() {
	// Determine logging level
	var level string = "ERROR"
	if v, ok := os.LookupEnv("LOG_LEVEL"); ok {
		level = v
	}

	// Initialize the logger
	log = common.NewLogger(level)
	log.Warn("%s LOGGING ENABLED (use LOG_LEVEL env var to configure)", common.LevelString(log.Level))

	log.Debug("AWS account ......................: %s", awsAccount)
	log.Debug("AWS region .......................: %s", awsRegion)
	log.Debug("S3 structured content bucket .....: %s", s3StructuredContentBucket)

	// AWS S3 client obj
	content = storage.Repository{
		Store:  s3.New(session.New(&aws.Config{Region: aws.String(awsRegion)})),
		Bucket: s3StructuredContentBucket,
	}

	// Elasticsearch topic indexer
	var err error
	var esConfig elasticsearch.Config
	var esClient *elasticsearch.Client

	esConfig = elasticsearch.Config{
		Addresses: []string{esEndpoint},
		Username:  esUsername,
		Password:  esPassword,
	}

	if esClient, err = elasticsearch.NewClient(esConfig); err == nil {
		topicSearch = &storage.ElasticTopicSearch{Client: esClient, IndexName: esIndex}
	} else {
		log.Error("Unable to create ElasticSearch client: %s", err)
	}

	recommendService = rosette.Rosette{APIKey: rosetteAPIKey, Logger: log}
}

func main() {
	lambda.Start(handleRequest)
}
