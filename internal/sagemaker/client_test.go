package sagemaker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	// Test with explicit region
	client, err := NewClient("us-west-2")
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Test with empty region (should use AWS SDK's default region resolution)
	client, err = NewClient("")
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Test with AWS_REGION environment variable
	os.Setenv("AWS_REGION", "us-east-1")
	client, err = NewClient("")
	assert.NoError(t, err)
	assert.NotNil(t, client)
	os.Unsetenv("AWS_REGION")
}

// MockSageMakerClient provides a mock implementation of the SageMaker client
type MockSageMakerClient struct {
	listAppsFunc         func(ctx context.Context, params *sagemaker.ListAppsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListAppsOutput, error)
	listEndpointsFunc    func(ctx context.Context, params *sagemaker.ListEndpointsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListEndpointsOutput, error)
	listNotebookFunc     func(ctx context.Context, params *sagemaker.ListNotebookInstancesInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListNotebookInstancesOutput, error)
	listDomainsFunc      func(ctx context.Context, params *sagemaker.ListDomainsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListDomainsOutput, error)
}

func (m *MockSageMakerClient) ListApps(ctx context.Context, params *sagemaker.ListAppsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListAppsOutput, error) {
	if m.listAppsFunc != nil {
		return m.listAppsFunc(ctx, params, optFns...)
	}
	return &sagemaker.ListAppsOutput{}, nil
}

func (m *MockSageMakerClient) ListEndpoints(ctx context.Context, params *sagemaker.ListEndpointsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListEndpointsOutput, error) {
	if m.listEndpointsFunc != nil {
		return m.listEndpointsFunc(ctx, params, optFns...)
	}
	return &sagemaker.ListEndpointsOutput{}, nil
}

func (m *MockSageMakerClient) ListNotebookInstances(ctx context.Context, params *sagemaker.ListNotebookInstancesInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListNotebookInstancesOutput, error) {
	if m.listNotebookFunc != nil {
		return m.listNotebookFunc(ctx, params, optFns...)
	}
	return &sagemaker.ListNotebookInstancesOutput{}, nil
}

func (m *MockSageMakerClient) ListDomains(ctx context.Context, params *sagemaker.ListDomainsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListDomainsOutput, error) {
	if m.listDomainsFunc != nil {
		return m.listDomainsFunc(ctx, params, optFns...)
	}
	return &sagemaker.ListDomainsOutput{}, nil
}

func TestListStudioApps_NilFields(t *testing.T) {
	// Prepare a context
	ctx := context.Background()

	// Create a mock client
	mockClient := &MockSageMakerClient{
		listAppsFunc: func(ctx context.Context, params *sagemaker.ListAppsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListAppsOutput, error) {
			return &sagemaker.ListAppsOutput{
				Apps: []types.AppDetails{
					{
						// Intentionally leave some fields nil
						Status:     types.AppStatusInService,
						AppType:    types.AppTypeJupyterServer,
						AppName:    nil,
						CreationTime: nil,
						UserProfileName: nil,
						ResourceSpec: nil,
					},
					{
						// Old Studio app with some fields populated
						Status:     types.AppStatusInService,
						AppType:    types.AppTypeJupyterServer,
						AppName:    aws.String("TestApp"),
						CreationTime: aws.Time(time.Now()),
						UserProfileName: aws.String("TestUser"),
						ResourceSpec: &types.ResourceSpec{
							InstanceType: types.AppInstanceType("ml.t3.medium"),
						},
					},
					{
						// New Studio app with space name
						Status:     types.AppStatusInService,
						AppType:    types.AppTypeJupyterLab,
						AppName:    aws.String("NewTestApp"),
						CreationTime: aws.Time(time.Now()),
						UserProfileName: aws.String("NewTestUser"),
						SpaceName:   aws.String("TestSpace"),
						ResourceSpec: &types.ResourceSpec{
							InstanceType: types.AppInstanceType("ml.t3.large"),
						},
					},
				},
			}, nil
		},
	}

	// Create a Client with the mock
	client := &Client{
		client: mockClient,
	}

	// Call the method
	resources, err := client.ListStudioApps(ctx)

	// Assert expectations
	assert.NoError(t, err)
	assert.Len(t, resources, 2, "Should include apps with non-nil names")
	
	// Verify the old Studio app details
	oldStudioApp := resources[0]
	assert.Equal(t, "TestApp", oldStudioApp.Name)
	assert.Equal(t, "TestUser", oldStudioApp.UserProfile)
	assert.Equal(t, "ml.t3.medium", oldStudioApp.InstanceType)
	assert.Equal(t, "JupyterServer", oldStudioApp.AppType)
	assert.Equal(t, "Old Studio (JupyterServer)", oldStudioApp.StudioType)
	assert.Empty(t, oldStudioApp.SpaceName)

	// Verify the new Studio app details
	newStudioApp := resources[1]
	assert.Equal(t, "NewTestApp", newStudioApp.Name)
	assert.Equal(t, "NewTestUser", newStudioApp.UserProfile)
	assert.Equal(t, "ml.t3.large", newStudioApp.InstanceType)
	assert.Equal(t, "JupyterLab", newStudioApp.AppType)
	assert.Equal(t, "New Studio (JupyterLab)", newStudioApp.StudioType)
	assert.Equal(t, "TestSpace", newStudioApp.SpaceName)
}

func TestListStudioApps_StatusHandling(t *testing.T) {
	// Prepare a context
	ctx := context.Background()

	// Create a mock client with mixed statuses
	mockClient := &MockSageMakerClient{
		listAppsFunc: func(ctx context.Context, params *sagemaker.ListAppsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListAppsOutput, error) {
			return &sagemaker.ListAppsOutput{
				Apps: []types.AppDetails{
					{
						// Running old Studio app
						Status:     types.AppStatusInService,
						AppType:    types.AppTypeJupyterServer,
						AppName:    aws.String("RunningOldApp"),
						CreationTime: aws.Time(time.Now()),
						UserProfileName: aws.String("OldUser"),
					},
					{
						// Stopped new Studio app
						Status:     types.AppStatusDeleted,
						AppType:    types.AppTypeJupyterLab,
						AppName:    aws.String("StoppedNewApp"),
						CreationTime: aws.Time(time.Now().Add(-1 * time.Hour)),
						UserProfileName: aws.String("NewUser"),
						SpaceName:   aws.String("StoppedSpace"),
					},
				},
			}, nil
		},
	}

	// Create a Client with the mock
	client := &Client{
		client: mockClient,
	}

	// Call the method
	resources, err := client.ListStudioApps(ctx)

	// Assert expectations
	assert.NoError(t, err)
	assert.Len(t, resources, 1, "Should only include InService apps")
	
	// Verify the running old Studio app details
	runningOldApp := resources[0]
	assert.Equal(t, "RunningOldApp", runningOldApp.Name)
	assert.Equal(t, "InService", runningOldApp.Status)
	assert.Equal(t, "Old Studio (JupyterServer)", runningOldApp.StudioType)
}

func TestConcurrentResourceListing(t *testing.T) {
	// Prepare a context
	ctx := context.Background()

	// Create a mock client with simulated delays
	mockClient := &MockSageMakerClient{
		listEndpointsFunc: func(ctx context.Context, params *sagemaker.ListEndpointsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListEndpointsOutput, error) {
			time.Sleep(100 * time.Millisecond) // Simulate some delay
			return &sagemaker.ListEndpointsOutput{
				Endpoints: []types.EndpointSummary{
					{
						EndpointName:     aws.String("Endpoint1"),
						EndpointStatus:   types.EndpointStatusInService,
						CreationTime:     aws.Time(time.Now().Add(-1 * time.Hour)),
					},
				},
			}, nil
		},
		listNotebookFunc: func(ctx context.Context, params *sagemaker.ListNotebookInstancesInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListNotebookInstancesOutput, error) {
			time.Sleep(50 * time.Millisecond) // Simulate some delay
			return &sagemaker.ListNotebookInstancesOutput{
				NotebookInstances: []types.NotebookInstanceSummary{
					{
						NotebookInstanceName:     aws.String("Notebook1"),
						NotebookInstanceStatus:   types.NotebookInstanceStatusInService,
						CreationTime:             aws.Time(time.Now().Add(-2 * time.Hour)),
					},
				},
			}, nil
		},
		listAppsFunc: func(ctx context.Context, params *sagemaker.ListAppsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListAppsOutput, error) {
			time.Sleep(75 * time.Millisecond) // Simulate some delay
			return &sagemaker.ListAppsOutput{
				Apps: []types.AppDetails{
					{
						AppName:     aws.String("App1"),
						Status:      types.AppStatusInService,
						CreationTime: aws.Time(time.Now().Add(-3 * time.Hour)),
					},
				},
			}, nil
		},
	}

	// Create a Client with the mock
	client := &Client{
		client: mockClient,
	}

	// Measure total time for concurrent calls
	startTime := time.Now()
	
	// Perform concurrent resource listing
	var wg sync.WaitGroup
	wg.Add(3)

	var endpointResults, notebookResults, appResults []ResourceInfo
	var endpointErr, notebookErr, appErr error

	go func() {
		defer wg.Done()
		endpointResults, endpointErr = client.ListEndpoints(ctx)
	}()

	go func() {
		defer wg.Done()
		notebookResults, notebookErr = client.ListNotebooks(ctx)
	}()

	go func() {
		defer wg.Done()
		appResults, appErr = client.ListStudioApps(ctx)
	}()

	wg.Wait()

	// Calculate total time
	totalTime := time.Since(startTime)

	// Assert no errors
	assert.NoError(t, endpointErr)
	assert.NoError(t, notebookErr)
	assert.NoError(t, appErr)

	// Assert results
	assert.Len(t, endpointResults, 1)
	assert.Len(t, notebookResults, 1)
	assert.Len(t, appResults, 1)

	// Total time should be less than sequential calls (sum of delays)
	// Allowing some buffer for goroutine overhead
	assert.Less(t, totalTime.Milliseconds(), int64(250), "Concurrent calls should be faster than sequential")
}

// createMockClient creates a new mock SageMaker client with the specified ListDomains behavior
func createMockClient(listDomainsFunc func(ctx context.Context, params *sagemaker.ListDomainsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListDomainsOutput, error)) *Client {
	return &Client{
		client: &MockSageMakerClient{
			listDomainsFunc: listDomainsFunc,
		},
		region: "us-west-2",
	}
}

// createAPIError creates a new smithy.GenericAPIError with the specified code and message
func createAPIError(code, message string) error {
	return &smithy.GenericAPIError{
		Code:    code,
		Message: message,
	}
}

func TestValidateConfiguration(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name           string
		mockResponse   interface{}
		mockError      error
		expectError    bool
		expectResource bool
		errorContains  string
	}{
		{
			name:           "Successful configuration",
			mockResponse:   &sagemaker.ListDomainsOutput{},
			expectResource: true,
		},
		{
			name:           "Access Denied",
			mockError:      createAPIError("AccessDeniedException", "Access Denied"),
			expectResource: false,
		},
		{
			name:           "Invalid Token",
			mockError:      createAPIError("InvalidClientTokenId", "Invalid Token"),
			expectResource: false,
		},
		{
			name:           "Expired Token",
			mockError:      createAPIError("ExpiredToken", "Token has expired"),
			expectResource: false,
		},
		{
			name:           "Signature Mismatch",
			mockError:      createAPIError("SignatureDoesNotMatch", "Signature validation failed"),
			expectResource: false,
		},
		{
			name:           "Throttling",
			mockError:      createAPIError("ThrottlingException", "Rate exceeded"),
			expectError:    true,
			errorContains:  "Rate exceeded",
			expectResource: false,
		},
		{
			name:           "Internal Failure",
			mockError:      createAPIError("InternalFailure", "Internal service error"),
			expectError:    true,
			errorContains:  "Internal service error",
			expectResource: false,
		},
		{
			name:           "IMDS Role Error",
			mockError:      fmt.Errorf("no EC2 IMDS role found"),
			expectError:    true,
			errorContains:  "No AWS role configured",
			expectResource: false,
		},
		{
			name:           "Empty Response",
			mockResponse:   &sagemaker.ListDomainsOutput{},
			expectResource: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := createMockClient(func(ctx context.Context, params *sagemaker.ListDomainsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListDomainsOutput, error) {
				if tc.mockError != nil {
					return nil, tc.mockError
				}
				return tc.mockResponse.(*sagemaker.ListDomainsOutput), nil
			})

			hasResources, err := mockClient.ValidateConfiguration(ctx)

			// Verify error expectations
			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify resource expectations
			assert.Equal(t, tc.expectResource, hasResources)

			// Test error suppression for authentication errors
			if tc.mockError != nil && !tc.expectError {
				// Second call should suppress the error
				hasResources, err = mockClient.ValidateConfiguration(ctx)
				assert.NoError(t, err)
				assert.False(t, hasResources)
			}
		})
	}
}

// TestErrorTracker tests the error suppression functionality
func TestErrorTracker(t *testing.T) {
	tracker := NewErrorTracker()

	// Test case 1: Basic error tracking
	err1 := fmt.Errorf("test error")
	trackedErr := tracker.Track(err1)
	assert.Error(t, trackedErr)
	assert.Equal(t, err1.Error(), trackedErr.Error())

	// Test case 2: Duplicate error suppression
	trackedErr = tracker.Track(err1)
	assert.Error(t, trackedErr)
	trackedErr = tracker.Track(err1)
	assert.Nil(t, trackedErr, "Third occurrence should be suppressed")

	// Test case 3: New error resets suppression
	err2 := fmt.Errorf("different error")
	trackedErr = tracker.Track(err2)
	assert.Error(t, trackedErr)
	assert.Equal(t, err2.Error(), trackedErr.Error())

	// Test case 4: Authentication error formatting
	authErr := fmt.Errorf("no EC2 IMDS role found")
	trackedErr = tracker.Track(authErr)
	assert.Error(t, trackedErr)
	assert.Contains(t, trackedErr.Error(), "AWS Authentication Error")
	assert.Contains(t, trackedErr.Error(), "No AWS role configured")
}
