// Copyright (c) 2022 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License-AGPL.txt in the project root for license information.

package controller

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/gitpod-io/gitpod/usage/pkg/db"
	"github.com/gitpod-io/gitpod/usage/pkg/db/dbtest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestUsageReconciler_ReconcileTimeRange(t *testing.T) {
	startOfMay := time.Date(2022, 05, 1, 0, 00, 00, 00, time.UTC)
	startOfJune := time.Date(2022, 06, 1, 0, 00, 00, 00, time.UTC)

	teamID := uuid.New()
	scenarioRunTime := time.Date(2022, 05, 31, 23, 00, 00, 00, time.UTC)

	instances := []db.WorkspaceInstance{
		// Ran throughout the reconcile period
		dbtest.NewWorkspaceInstance(t, db.WorkspaceInstance{
			ID:                 uuid.New(),
			UsageAttributionID: db.NewTeamAttributionID(teamID.String()),
			CreationTime:       db.NewVarcharTime(time.Date(2022, 05, 1, 00, 00, 00, 00, time.UTC)),
			StartedTime:        db.NewVarcharTime(time.Date(2022, 05, 1, 00, 00, 00, 00, time.UTC)),
			StoppedTime:        db.NewVarcharTime(time.Date(2022, 06, 1, 1, 0, 0, 0, time.UTC)),
		}),
		// Still running
		dbtest.NewWorkspaceInstance(t, db.WorkspaceInstance{
			ID:                 uuid.New(),
			UsageAttributionID: db.NewTeamAttributionID(teamID.String()),
			CreationTime:       db.NewVarcharTime(time.Date(2022, 05, 30, 00, 00, 00, 00, time.UTC)),
			StartedTime:        db.NewVarcharTime(time.Date(2022, 05, 30, 00, 00, 00, 00, time.UTC)),
		}),
		// No creation time, invalid record
		dbtest.NewWorkspaceInstance(t, db.WorkspaceInstance{
			ID:                 uuid.New(),
			UsageAttributionID: db.NewTeamAttributionID(teamID.String()),
			StartedTime:        db.NewVarcharTime(time.Date(2022, 06, 1, 1, 0, 0, 0, time.UTC)),
			StoppedTime:        db.NewVarcharTime(time.Date(2022, 06, 1, 1, 0, 0, 0, time.UTC)),
		}),
	}

	conn := dbtest.ConnectForTests(t)
	dbtest.CreateWorkspaceInstances(t, conn, instances...)

	reconciler := &UsageReconciler{
		billingController: &NoOpBillingController{},
		nowFunc:           func() time.Time { return scenarioRunTime },
		conn:              conn,
		pricer:            DefaultWorkspacePricer,
	}
	status, report, err := reconciler.ReconcileTimeRange(context.Background(), startOfMay, startOfJune)
	require.NoError(t, err)

	require.Len(t, report, 2)
	require.Equal(t, &UsageReconcileStatus{
		StartTime:                 startOfMay,
		EndTime:                   startOfJune,
		WorkspaceInstances:        2,
		InvalidWorkspaceInstances: 1,
	}, status)
}

func TestUsageReport_CreditSummaryForTeams(t *testing.T) {
	teamID := uuid.New().String()
	teamAttributionID := db.NewTeamAttributionID(teamID)

	scenarios := []struct {
		Name     string
		Report   UsageReport
		Expected map[string]int64
	}{
		{
			Name:     "no instances in report, no summary",
			Report:   []db.WorkspaceInstanceUsage{},
			Expected: map[string]int64{},
		},
		{
			Name: "skips user attributions",
			Report: []db.WorkspaceInstanceUsage{
				{
					AttributionID: db.NewUserAttributionID(uuid.New().String()),
				},
			},
			Expected: map[string]int64{},
		},
		{
			Name: "two workspace instances",
			Report: []db.WorkspaceInstanceUsage{
				{
					// has 1 day and 23 hours of usage
					AttributionID:  teamAttributionID,
					WorkspaceClass: defaultWorkspaceClass,
					StartedAt:      time.Date(2022, 05, 30, 00, 00, 00, 00, time.UTC),
					StoppedAt: sql.NullTime{
						Time:  time.Date(2022, 06, 1, 1, 0, 0, 0, time.UTC),
						Valid: true,
					},
					CreditsUsed: (24 + 23) * 10,
				},
				{
					// has 1 hour of usage
					AttributionID:  teamAttributionID,
					WorkspaceClass: defaultWorkspaceClass,
					StartedAt:      time.Date(2022, 05, 30, 00, 00, 00, 00, time.UTC),
					StoppedAt: sql.NullTime{
						Time:  time.Date(2022, 05, 30, 1, 0, 0, 0, time.UTC),
						Valid: true,
					},
					CreditsUsed: 10,
				},
			},
			Expected: map[string]int64{
				// total of 2 days runtime, at 10 credits per hour, that's 480 credits
				teamID: 480,
			},
		},
		{
			Name: "unknown workspace class uses default",
			Report: []db.WorkspaceInstanceUsage{
				// has 1 hour of usage
				{
					WorkspaceClass: "yolo-workspace-class",
					AttributionID:  teamAttributionID,
					StartedAt:      time.Date(2022, 05, 30, 00, 00, 00, 00, time.UTC),
					StoppedAt: sql.NullTime{
						Time:  time.Date(2022, 05, 30, 1, 0, 0, 0, time.UTC),
						Valid: true,
					},
					CreditsUsed: 10,
				},
			},
			Expected: map[string]int64{
				// total of 1 hour usage, at default cost of 10 credits per hour
				teamID: 10,
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			actual := s.Report.CreditSummaryForTeams()
			require.Equal(t, s.Expected, actual)
		})
	}
}

func TestInstanceToUsageRecords(t *testing.T) {
	maxStopTime := time.Date(2022, 05, 31, 23, 00, 00, 00, time.UTC)
	teamID, ownerID, projectID := uuid.New().String(), uuid.New(), uuid.New()
	workspaceID := dbtest.GenerateWorkspaceID()
	teamAttributionID := db.NewTeamAttributionID(teamID)
	instanceId := uuid.New()
	creationTime := db.NewVarcharTime(time.Date(2022, 05, 30, 00, 00, 00, 00, time.UTC))
	stoppedTime := db.NewVarcharTime(time.Date(2022, 06, 1, 1, 0, 0, 0, time.UTC))

	scenarios := []struct {
		Name     string
		Records  []db.WorkspaceInstanceForUsage
		Expected []db.WorkspaceInstanceUsage
	}{
		{
			Name: "a stopped workspace instance",
			Records: []db.WorkspaceInstanceForUsage{
				{
					ID:                 instanceId,
					WorkspaceID:        workspaceID,
					OwnerID:            ownerID,
					ProjectID:          sql.NullString{},
					WorkspaceClass:     defaultWorkspaceClass,
					Type:               db.WorkspaceType_Prebuild,
					UsageAttributionID: teamAttributionID,
					CreationTime:       creationTime,
					StoppedTime:        stoppedTime,
				},
			},
			Expected: []db.WorkspaceInstanceUsage{{
				InstanceID:     instanceId,
				AttributionID:  teamAttributionID,
				UserID:         ownerID,
				WorkspaceID:    workspaceID,
				ProjectID:      "",
				WorkspaceType:  db.WorkspaceType_Prebuild,
				WorkspaceClass: defaultWorkspaceClass,
				CreditsUsed:    470,
				StartedAt:      creationTime.Time(),
				StoppedAt:      sql.NullTime{Time: stoppedTime.Time(), Valid: true},
				GenerationID:   0,
				Deleted:        false,
			}},
		},
		{
			Name: "workspace instance that is still running",
			Records: []db.WorkspaceInstanceForUsage{
				{
					ID:                 instanceId,
					OwnerID:            ownerID,
					ProjectID:          sql.NullString{String: projectID.String(), Valid: true},
					WorkspaceClass:     defaultWorkspaceClass,
					Type:               db.WorkspaceType_Regular,
					WorkspaceID:        workspaceID,
					UsageAttributionID: teamAttributionID,
					CreationTime:       creationTime,
					StoppedTime:        db.VarcharTime{},
				},
			},
			Expected: []db.WorkspaceInstanceUsage{{
				InstanceID:     instanceId,
				AttributionID:  teamAttributionID,
				UserID:         ownerID,
				ProjectID:      projectID.String(),
				WorkspaceID:    workspaceID,
				WorkspaceType:  db.WorkspaceType_Regular,
				StartedAt:      creationTime.Time(),
				StoppedAt:      sql.NullTime{},
				WorkspaceClass: defaultWorkspaceClass,
				CreditsUsed:    470,
			}},
		},
	}

	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			actual := instancesToUsageRecords(s.Records, DefaultWorkspacePricer, maxStopTime)
			require.Equal(t, s.Expected, actual)
		})
	}
}
