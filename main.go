package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

func isSnapshotTagged(tagKey string, tags []*rds.Tag) bool {
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == tagKey {
			return true
		}
	}
	return false
}

func removeOldSnapshots(svc *rds.RDS, retentionDays int) {
	dbInput := (&rds.DescribeDBClusterSnapshotsInput{}).SetFilters([]*rds.Filter{
		&rds.Filter{
			Name:   aws.String("snapshot-type"),
			Values: []*string{aws.String("manual")},
		},
	})

	result, err := svc.DescribeDBClusterSnapshots(dbInput)
	if err != nil {
		exitErrorf("Unable to list snapshots, %v", err)
	}

	currentTime := time.Now()

	for _, s := range result.DBClusterSnapshots {
		timeDiff := currentTime.Sub(*s.SnapshotCreateTime)

		result, err := svc.ListTagsForResource(&rds.ListTagsForResourceInput{
			ResourceName: s.DBClusterSnapshotArn,
		})
		if err != nil {
			exitErrorf("Unable to get tags for snapshot, %v", err)
		}

		if int(timeDiff.Hours()/24) >= retentionDays && isSnapshotTagged("lambda_automatic", result.TagList) {
			fmt.Printf("Deleting snapshot %s from %s\n",
				aws.StringValue(s.DBClusterSnapshotIdentifier), s.SnapshotCreateTime.Format("2006-01-02 15:04:05"))

			_, err := svc.DeleteDBClusterSnapshot(&rds.DeleteDBClusterSnapshotInput{
				DBClusterSnapshotIdentifier: s.DBClusterSnapshotIdentifier,
			})
			if err != nil {
				exitErrorf("Unable to delete snapshot, %v", err)
			}
		}
	}
}

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

		snapshot, err := svc.CopyDBClusterSnapshot(&rds.CopyDBClusterSnapshotInput{
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

		_, err = svc.AddTagsToResource((&rds.AddTagsToResourceInput{
			ResourceName: snapshot.DBClusterSnapshot.DBClusterSnapshotArn,
		}).SetTags(
			[]*rds.Tag{
				&rds.Tag{
					Key:   aws.String("lambda_automatic"),
					Value: aws.String("true"),
				},
			},
		))
		if err != nil {
			exitErrorf("Error tagging snapshot, %v", err)
		}

		retentionDays, err := strconv.Atoi(os.Getenv("RETENTION_DAYS"))
		if err != nil {
			log.Fatal("Error parsing RETENTION_DAYS env var")
		}

		removeOldSnapshots(svc, retentionDays)
	}
}

func main() {
	lambda.Start(HandleRequest)
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
