package eksapi

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-k8s-tester/kubetest2/internal/awssdk"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudformationtypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"k8s.io/klog/v2"
)

func NewJanitor(maxResourceAge time.Duration) *janitor {
	awsConfig := awssdk.NewConfig()
	return &janitor{
		maxResourceAge: maxResourceAge,
		awsConfig:      awsConfig,
		cfnClient:      cloudformation.NewFromConfig(awsConfig),
	}
}

type janitor struct {
	maxResourceAge time.Duration
	awsConfig      aws.Config
	cfnClient      *cloudformation.Client
}

func (j *janitor) Sweep(ctx context.Context) error {
	stacks := cloudformation.NewDescribeStacksPaginator(j.cfnClient, &cloudformation.DescribeStacksInput{})
	for stacks.HasMorePages() {
		page, err := stacks.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, stack := range page.Stacks {
			resourceID := *stack.StackName
			if !strings.HasPrefix(resourceID, ResourcePrefix) {
				continue
			}
			if stack.StackStatus == "DELETE_COMPLETE" {
				continue
			}
			resourceAge := time.Since(*stack.CreationTime)
			if resourceAge < j.maxResourceAge {
				klog.Infof("skipping resources (%v old): %s", resourceAge, resourceID)
				continue
			}
			clients := j.awsClientsForStack(stack)
			klog.Infof("deleting resources (%v old): %s", resourceAge, resourceID)
			if err := deleteResources(clients, resourceID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (j *janitor) awsClientsForStack(stack cloudformationtypes.Stack) *awsClients {
	var eksEndpointURL string
	for _, tag := range stack.Tags {
		if *tag.Key == eksEndpointURLTag {
			eksEndpointURL = *tag.Value
		}
	}
	return newAWSClients(j.awsConfig, eksEndpointURL)
}
