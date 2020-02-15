package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

func HandleRequest(ctx context.Context, snsEvent events.SNSEvent) {

	region := os.Getenv("REGION")

	for _, record := range snsEvent.Records {
		snsRecord := record.SNS
		snapshotIdentifier := snsRecord.MessageAttributes["snapshot_identifier"].(map[string]interface{})["Value"].(string)
		snapshotArn := snsRecord.MessageAttributes["snapshot_arn"].(map[string]interface{})["Value"].(string)

		fmt.Printf("[%s %s] Message = %s\n", record.EventSource, snsRecord.Timestamp, snsRecord.Message)

		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region)},
		)

		svc := rds.New(sess)

		fmt.Printf("Copying snapshot %s\n", snapshotArn)

		_, err = svc.CopyDBClusterSnapshot(&rds.CopyDBClusterSnapshotInput{
			SourceDBClusterSnapshotIdentifier: aws.String(snapshotArn),
			TargetDBClusterSnapshotIdentifier: aws.String(snapshotIdentifier),
			KmsKeyId:                          aws.String(os.Getenv("KMS_KEY_ID")),
		})
		if err != nil {
			exitErrorf("Error occurred while copying snapshot in cluster, %v %v", snapshotIdentifier, err)
		}

		err = svc.WaitUntilDBClusterSnapshotAvailable(&rds.DescribeDBClusterSnapshotsInput{
			DBClusterSnapshotIdentifier: aws.String(snapshotIdentifier),
		})
		if err != nil {
			exitErrorf("Error occurred while waiting for snapshot to be created in cluster, %v %v", snapshotArn, err)
		}

	}
}

func main() {
	lambda.Start(HandleRequest)
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
