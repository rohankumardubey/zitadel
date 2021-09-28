package projection

import (
	"testing"

	"github.com/caos/zitadel/internal/domain"
	"github.com/caos/zitadel/internal/errors"
	"github.com/caos/zitadel/internal/eventstore"
	"github.com/caos/zitadel/internal/eventstore/handler"
	"github.com/caos/zitadel/internal/eventstore/repository"
	"github.com/caos/zitadel/internal/repository/org"
)

func TestOrgProjection_reduces(t *testing.T) {
	type args struct {
		event func(t *testing.T) eventstore.EventReader
	}
	tests := []struct {
		name   string
		args   args
		reduce func(event eventstore.EventReader) (*handler.Statement, error)
		want   wantReduce
	}{
		{
			name: "reducePrimaryDomainSet",
			args: args{
				event: getEvent(testEvent(
					repository.EventType(org.OrgDomainPrimarySetEventType),
					org.AggregateType,
					[]byte(`{"domain": "domain.new"}`),
				), org.DomainPrimarySetEventMapper),
			},
			reduce: (&OrgProjection{}).reducePrimaryDomainSet,
			want: wantReduce{
				aggregateType:    eventstore.AggregateType("org"),
				sequence:         15,
				previousSequence: 10,
				projectionName:   OrgProjectionTable,
				executer: &testExecuter{
					executions: []execution{
						{
							expectedStmt: "UPDATE zitadel.projections.orgs SET (change_date, sequence, primary_domain) = ($1, $2, $3) WHERE (id = $4)",
							expectedArgs: []interface{}{
								anyArg{},
								uint64(15),
								"domain.new",
								"agg-id",
							},
						},
					},
				},
			},
		},
		{
			name: "reduceOrgReactivated",
			args: args{
				event: getEvent(testEvent(
					repository.EventType(org.OrgReactivatedEventType),
					org.AggregateType,
					nil,
				), org.OrgReactivatedEventMapper),
			},
			reduce: (&OrgProjection{}).reduceOrgReactivated,
			want: wantReduce{
				aggregateType:    eventstore.AggregateType("org"),
				sequence:         15,
				previousSequence: 10,
				projectionName:   OrgProjectionTable,
				executer: &testExecuter{
					executions: []execution{
						{
							expectedStmt: "UPDATE zitadel.projections.orgs SET (change_date, sequence, org_state) = ($1, $2, $3) WHERE (id = $4)",
							expectedArgs: []interface{}{
								anyArg{},
								uint64(15),
								domain.OrgStateActive,
								"agg-id",
							},
						},
					},
				},
			},
		},
		{
			name: "reduceOrgDeactivated",
			args: args{
				event: getEvent(testEvent(
					repository.EventType(org.OrgDeactivatedEventType),
					org.AggregateType,
					nil,
				), org.OrgDeactivatedEventMapper),
			},
			reduce: (&OrgProjection{}).reduceOrgDeactivated,
			want: wantReduce{
				aggregateType:    eventstore.AggregateType("org"),
				sequence:         15,
				previousSequence: 10,
				projectionName:   OrgProjectionTable,
				executer: &testExecuter{
					executions: []execution{
						{
							expectedStmt: "UPDATE zitadel.projections.orgs SET (change_date, sequence, org_state) = ($1, $2, $3) WHERE (id = $4)",
							expectedArgs: []interface{}{
								anyArg{},
								uint64(15),
								domain.OrgStateInactive,
								"agg-id",
							},
						},
					},
				},
			},
		},
		{
			name: "reduceOrgChanged",
			args: args{
				event: getEvent(testEvent(
					repository.EventType(org.OrgChangedEventType),
					org.AggregateType,
					[]byte(`{"name": "new name"}`),
				), org.OrgChangedEventMapper),
			},
			reduce: (&OrgProjection{}).reduceOrgChanged,
			want: wantReduce{
				aggregateType:    eventstore.AggregateType("org"),
				sequence:         15,
				previousSequence: 10,
				projectionName:   OrgProjectionTable,
				executer: &testExecuter{
					executions: []execution{
						{
							expectedStmt: "UPDATE zitadel.projections.orgs SET (change_date, sequence, name) = ($1, $2, $3) WHERE (id = $4)",
							expectedArgs: []interface{}{
								anyArg{},
								uint64(15),
								"new name",
								"agg-id",
							},
						},
					},
				},
			},
		},
		{
			name: "reduceOrgChanged no changes",
			args: args{
				event: getEvent(testEvent(
					repository.EventType(org.OrgChangedEventType),
					org.AggregateType,
					[]byte(`{}`),
				), org.OrgChangedEventMapper),
			},
			reduce: (&OrgProjection{}).reduceOrgChanged,
			want: wantReduce{
				aggregateType:    eventstore.AggregateType("org"),
				sequence:         15,
				previousSequence: 10,
				projectionName:   OrgProjectionTable,
				executer:         &testExecuter{},
			},
		},
		{
			name: "reduceOrgAdded",
			args: args{
				event: getEvent(testEvent(
					repository.EventType(org.OrgAddedEventType),
					org.AggregateType,
					[]byte(`{"name": "name"}`),
				), org.OrgAddedEventMapper),
			},
			reduce: (&OrgProjection{}).reduceOrgAdded,
			want: wantReduce{
				aggregateType:    eventstore.AggregateType("org"),
				sequence:         15,
				previousSequence: 10,
				projectionName:   OrgProjectionTable,
				executer: &testExecuter{
					executions: []execution{
						{
							expectedStmt: "INSERT INTO zitadel.projections.orgs (id, creation_date, change_date, resource_owner, sequence, name, org_state) VALUES ($1, $2, $3, $4, $5, $6, $7)",
							expectedArgs: []interface{}{
								"agg-id",
								anyArg{},
								anyArg{},
								"ro-id",
								uint64(15),
								"name",
								domain.OrgStateActive,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := baseEvent(t)
			got, err := tt.reduce(event)
			if _, ok := err.(errors.InvalidArgument); !ok {
				t.Errorf("no wrong event mapping: %v, got: %v", err, got)
			}

			event = tt.args.event(t)
			got, err = tt.reduce(event)
			assertReduce(t, got, err, tt.want)
		})
	}
}
