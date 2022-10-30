package store

import (
	"github.com/G-Research/armada/internal/common/ingest"
	"github.com/G-Research/armada/internal/eventingester/configuration"
	"github.com/G-Research/armada/internal/eventingester/model"
	"github.com/hashicorp/go-multierror"
	log "github.com/sirupsen/logrus"
	"regexp"

	"github.com/go-redis/redis"
)

const (
	eventStreamPrefix = "Events:"
	dataKey           = "message"
)

type RedisEventStore struct {
	db                     redis.UniversalClient
	eventRetention         configuration.EventRetentionPolicy
	maxRetryBackoffSeconds int
	maxRows                int
	maxSize                int
	fatalErrors            []*regexp.Regexp
}

func NewRedisEventStore(db redis.UniversalClient, eventRetention configuration.EventRetentionPolicy, fatalErrors []*regexp.Regexp) ingest.Sink[*model.BatchUpdate] {
	return &RedisEventStore{db: db, eventRetention: eventRetention, fatalErrors: fatalErrors}
}

func (repo *RedisEventStore) Store(update *model.BatchUpdate) error {
	if len(update.Events) == 0 {
		return nil
	}
	var result *multierror.Error

	// Insert such that we never send more than maxRows rows or maxSize of data to redis at a time
	currentSize := 0
	currentRows := 0
	batch := make([]*model.Event, 0, repo.maxRows)

	for i, event := range update.Events {
		newSize := currentSize + len(event.Event)
		newRows := currentRows + 1
		if newSize > repo.maxSize || newRows > repo.maxRows {
			err := repo.doStore(batch)
			multierror.Append(result, err)
			batch = make([]*model.Event, 0, repo.maxRows)
			currentSize = 0
			currentRows = 0
		}
		batch = append(batch, event)
		currentSize += len(event.Event)
		currentRows++

		// If this is the last element we need to flush
		if i == len(update.Events)-1 {
			err := repo.doStore(batch)
			multierror.Append(result, err)
		}
	}
	return result.ErrorOrNil()
}

func (repo *RedisEventStore) doStore(update []*model.Event) error {

	type eventData struct {
		key  string
		data []byte
	}
	var data []eventData
	uniqueJobSets := make(map[string]bool)

	for _, e := range update {
		key := getJobSetEventsKey(e.Queue, e.Jobset)
		data = append(data, eventData{key: key, data: e.Event})
		uniqueJobSets[key] = true
	}

	pipe := repo.db.Pipeline()
	for _, e := range data {
		pipe.XAdd(&redis.XAddArgs{
			Stream: e.key,
			Values: map[string]interface{}{
				dataKey: e.data,
			},
		})
	}

	if repo.eventRetention.ExpiryEnabled {
		for key := range uniqueJobSets {
			pipe.Expire(key, repo.eventRetention.RetentionDuration)
		}
	}

	return ingest.WithRetry(func() (error, bool) {
		_, err := pipe.Exec()
		if err == nil {
			return nil, false
		} else {
			return err, repo.isRetryableRedisError(err)
		}
	}, repo.maxRetryBackoffSeconds)
}

// IsRetryableRedisError returns true if the error doesn't match the list of nonRetryableErrors
func (repo *RedisEventStore) isRetryableRedisError(err error) bool {
	if err == nil {
		return true
	}
	s := err.Error()
	for _, r := range repo.fatalErrors {
		if r.MatchString(s) {
			log.Infof("Error %s matched regex %s and so will be considered fatal", s, r)
			return false
		}
	}
	return true
}

func getJobSetEventsKey(queue, jobSetId string) string {
	return eventStreamPrefix + queue + ":" + jobSetId
}
