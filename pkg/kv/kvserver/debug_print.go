// Copyright 2018 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package kvserver

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/kv/kvserver/kvserverpb"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/storage"
	"github.com/cockroachdb/cockroach/pkg/storage/enginepb"
	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/cockroachdb/errors"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

// PrintEngineKeyValue attempts to print the given key-value pair to
// os.Stdout, utilizing SprintMVCCKeyValue in the case of an MVCCKeyValue.
func PrintEngineKeyValue(k storage.EngineKey, v []byte) {
	fmt.Println(SprintEngineKeyValue(k, v))
}

// PrintMVCCKeyValue attempts to pretty-print the specified MVCCKeyValue to
// os.Stdout, falling back to '%q' formatting.
func PrintMVCCKeyValue(kv storage.MVCCKeyValue) {
	fmt.Println(SprintMVCCKeyValue(kv, true /* printKey */))
}

// SprintEngineKey pretty-prints the specified EngineKey, using the correct
// MVCC or Lock Table version formatting.
func SprintEngineKey(key storage.EngineKey) string {
	if key.IsMVCCKey() {
		if mvccKey, err := key.ToMVCCKey(); err == nil {
			return SprintMVCCKey(mvccKey)
		}
	}

	return fmt.Sprintf("%s %x (%#x): ", key.Key, key.Version, key.Encode())
}

// SprintMVCCKey pretty-prints the specified MVCCKey.
func SprintMVCCKey(key storage.MVCCKey) string {
	return fmt.Sprintf("%s %s (%#x): ", key.Timestamp, key.Key, storage.EncodeMVCCKey(key))
}

// SprintEngineKeyValue is like PrintEngineKeyValue, but returns a string.  In
// the case of an MVCCKey, it will utilize SprintMVCCKeyValue for proper MVCC
// formatting.
func SprintEngineKeyValue(k storage.EngineKey, v []byte) string {
	if k.IsMVCCKey() {
		if key, err := k.ToMVCCKey(); err == nil {
			return SprintMVCCKeyValue(storage.MVCCKeyValue{Key: key, Value: v}, true /* printKey */)
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %x (%#x): ", k.Key, k.Version, k.Encode())
	if out, err := tryIntent(storage.MVCCKeyValue{Value: v}); err == nil {
		sb.WriteString(out)
	} else {
		fmt.Fprintf(&sb, "%x", v)
	}
	return sb.String()
}

// DebugSprintMVCCKeyValueDecoders allows injecting alternative debug decoders.
var DebugSprintMVCCKeyValueDecoders []func(kv storage.MVCCKeyValue) (string, error)

// SprintMVCCKeyValue is like PrintMVCCKeyValue, but returns a string. If
// printKey is true, prints the key and the value together; otherwise,
// prints just the value.
func SprintMVCCKeyValue(kv storage.MVCCKeyValue, printKey bool) string {
	var sb strings.Builder
	if printKey {
		sb.WriteString(SprintMVCCKey(kv.Key))
	}

	decoders := append(DebugSprintMVCCKeyValueDecoders,
		tryRaftLogEntry,
		tryRangeDescriptor,
		tryMeta,
		tryTxn,
		tryRangeIDKey,
		tryTimeSeries,
		tryIntent,
		func(kv storage.MVCCKeyValue) (string, error) {
			// No better idea, just print raw bytes and hope that folks use `less -S`.
			return fmt.Sprintf("%q", kv.Value), nil
		},
	)

	for _, decoder := range decoders {
		out, err := decoder(kv)
		if err != nil {
			continue
		}
		sb.WriteString(out)
		return sb.String()
	}
	panic("unreachable")
}

// SprintIntent pretty-prints the specified intent value.
func SprintIntent(value []byte) string {
	if out, err := tryIntent(storage.MVCCKeyValue{Value: value}); err == nil {
		return out
	}
	return fmt.Sprintf("%x", value)
}

func tryRangeDescriptor(kv storage.MVCCKeyValue) (string, error) {
	if err := IsRangeDescriptorKey(kv.Key); err != nil {
		return "", err
	}
	var desc roachpb.RangeDescriptor
	if err := getProtoValue(kv.Value, &desc); err != nil {
		return "", err
	}
	return descStr(desc), nil
}

// tryIntent does not look at the key.
func tryIntent(kv storage.MVCCKeyValue) (string, error) {
	if len(kv.Value) == 0 {
		return "", errors.New("empty")
	}
	var meta enginepb.MVCCMetadata
	if err := protoutil.Unmarshal(kv.Value, &meta); err != nil {
		return "", err
	}
	s := fmt.Sprintf("%+v", &meta)
	if meta.Txn != nil {
		s = meta.Txn.WriteTimestamp.String() + " " + s
	}
	return s, nil
}

func decodeWriteBatch(writeBatch *kvserverpb.WriteBatch) (string, error) {
	if writeBatch == nil {
		return "<nil>\n", nil
	}

	r, err := storage.NewRocksDBBatchReader(writeBatch.Data)
	if err != nil {
		return "", err
	}

	// NB: always return sb.String() as the first arg, even on error, to give
	// the caller all the info we have (in case the writebatch is corrupted).
	var sb strings.Builder
	for r.Next() {
		switch r.BatchType() {
		case storage.BatchTypeDeletion:
			engineKey, err := r.EngineKey()
			if err != nil {
				return sb.String(), err
			}
			sb.WriteString(fmt.Sprintf("Delete: %s\n", SprintEngineKey(engineKey)))
		case storage.BatchTypeValue:
			engineKey, err := r.EngineKey()
			if err != nil {
				return sb.String(), err
			}
			sb.WriteString(fmt.Sprintf("Put: %s\n", SprintEngineKeyValue(engineKey, r.Value())))
		case storage.BatchTypeMerge:
			engineKey, err := r.EngineKey()
			if err != nil {
				return sb.String(), err
			}
			sb.WriteString(fmt.Sprintf("Merge: %s\n", SprintEngineKeyValue(engineKey, r.Value())))
		case storage.BatchTypeSingleDeletion:
			engineKey, err := r.EngineKey()
			if err != nil {
				return sb.String(), err
			}
			sb.WriteString(fmt.Sprintf("Single Delete: %s\n", SprintEngineKey(engineKey)))
		case storage.BatchTypeRangeDeletion:
			engineStartKey, err := r.EngineKey()
			if err != nil {
				return sb.String(), err
			}
			engineEndKey, err := r.EngineEndKey()
			if err != nil {
				return sb.String(), err
			}
			sb.WriteString(fmt.Sprintf(
				"Delete Range: [%s, %s)\n", SprintEngineKey(engineStartKey), SprintEngineKey(engineEndKey),
			))
		default:
			sb.WriteString(fmt.Sprintf("unsupported batch type: %d\n", r.BatchType()))
		}
	}
	return sb.String(), r.Error()
}

func tryRaftLogEntry(kv storage.MVCCKeyValue) (string, error) {
	var ent raftpb.Entry
	if err := maybeUnmarshalInline(kv.Value, &ent); err != nil {
		return "", err
	}

	var cmd kvserverpb.RaftCommand
	switch ent.Type {
	case raftpb.EntryNormal:
		if len(ent.Data) == 0 {
			return fmt.Sprintf("%s: EMPTY\n", &ent), nil
		}
		_, cmdData := DecodeRaftCommand(ent.Data)
		if err := protoutil.Unmarshal(cmdData, &cmd); err != nil {
			return "", err
		}
	case raftpb.EntryConfChange, raftpb.EntryConfChangeV2:
		var c raftpb.ConfChangeI
		if ent.Type == raftpb.EntryConfChange {
			var cc raftpb.ConfChange
			if err := protoutil.Unmarshal(ent.Data, &cc); err != nil {
				return "", err
			}
			c = cc
		} else {
			var cc raftpb.ConfChangeV2
			if err := protoutil.Unmarshal(ent.Data, &cc); err != nil {
				return "", err
			}
			c = cc
		}

		var ctx ConfChangeContext
		if err := protoutil.Unmarshal(c.AsV2().Context, &ctx); err != nil {
			return "", err
		}
		if err := protoutil.Unmarshal(ctx.Payload, &cmd); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown log entry type: %s", &ent)
	}
	ent.Data = nil

	var leaseStr string
	if l := cmd.DeprecatedProposerLease; l != nil {
		leaseStr = l.String() // use full lease, if available
	} else {
		leaseStr = fmt.Sprintf("lease #%d", cmd.ProposerLeaseSequence)
	}

	wbStr, err := decodeWriteBatch(cmd.WriteBatch)
	if err != nil {
		wbStr = "failed to decode: " + err.Error() + "\nafter:\n" + wbStr
	}
	cmd.WriteBatch = nil

	return fmt.Sprintf("%s by %s\n%s\nwrite batch:\n%s", &ent, leaseStr, &cmd, wbStr), nil
}

func tryTxn(kv storage.MVCCKeyValue) (string, error) {
	var txn roachpb.Transaction
	if err := maybeUnmarshalInline(kv.Value, &txn); err != nil {
		return "", err
	}
	return txn.String() + "\n", nil
}

func tryRangeIDKey(kv storage.MVCCKeyValue) (string, error) {
	if !kv.Key.Timestamp.IsEmpty() {
		return "", fmt.Errorf("range ID keys shouldn't have timestamps: %s", kv.Key)
	}
	_, _, suffix, _, err := keys.DecodeRangeIDKey(kv.Key.Key)
	if err != nil {
		return "", err
	}

	// All range ID keys are stored inline on the metadata.
	var meta enginepb.MVCCMetadata
	if err := protoutil.Unmarshal(kv.Value, &meta); err != nil {
		return "", err
	}
	value := roachpb.Value{RawBytes: meta.RawBytes}

	// Values encoded as protobufs set msg and continue outside the
	// switch. Other types are handled inside the switch and return.
	var msg protoutil.Message
	switch {
	case bytes.Equal(suffix, keys.LocalAbortSpanSuffix):
		msg = &roachpb.AbortSpanEntry{}

	case bytes.Equal(suffix, keys.LocalRangeGCThresholdSuffix):
		msg = &hlc.Timestamp{}

	case bytes.Equal(suffix, keys.LocalRangeVersionSuffix):
		msg = &roachpb.Version{}

	case bytes.Equal(suffix, keys.LocalRangeTombstoneSuffix):
		msg = &roachpb.RangeTombstone{}

	case bytes.Equal(suffix, keys.LocalRaftTruncatedStateSuffix):
		msg = &roachpb.RaftTruncatedState{}

	case bytes.Equal(suffix, keys.LocalRangeLeaseSuffix):
		msg = &roachpb.Lease{}

	case bytes.Equal(suffix, keys.LocalRangeAppliedStateSuffix):
		msg = &enginepb.RangeAppliedState{}

	case bytes.Equal(suffix, keys.LocalRangeStatsLegacySuffix):
		msg = &enginepb.MVCCStats{}

	case bytes.Equal(suffix, keys.LocalRaftHardStateSuffix):
		msg = &raftpb.HardState{}

	case bytes.Equal(suffix, keys.LocalRangeLastReplicaGCTimestampSuffix):
		msg = &hlc.Timestamp{}

	default:
		return "", fmt.Errorf("unknown raft id key %s", suffix)
	}

	if err := value.GetProto(msg); err != nil {
		return "", err
	}
	return msg.String(), nil
}

func tryMeta(kv storage.MVCCKeyValue) (string, error) {
	if !bytes.HasPrefix(kv.Key.Key, keys.Meta1Prefix) && !bytes.HasPrefix(kv.Key.Key, keys.Meta2Prefix) {
		return "", errors.New("not a meta key")
	}
	value := roachpb.Value{
		Timestamp: kv.Key.Timestamp,
		RawBytes:  kv.Value,
	}
	var desc roachpb.RangeDescriptor
	if err := value.GetProto(&desc); err != nil {
		return "", err
	}
	return descStr(desc), nil
}

func tryTimeSeries(kv storage.MVCCKeyValue) (string, error) {
	if len(kv.Value) == 0 || !bytes.HasPrefix(kv.Key.Key, keys.TimeseriesPrefix) {
		return "", errors.New("empty or not TS")
	}
	var meta enginepb.MVCCMetadata
	if err := protoutil.Unmarshal(kv.Value, &meta); err != nil {
		return "", err
	}
	v := roachpb.Value{RawBytes: meta.RawBytes}
	var ts roachpb.InternalTimeSeriesData
	if err := v.GetProto(&ts); err != nil {
		return "", err
	}
	return fmt.Sprintf("%+v [mergeTS=%s]", &ts, meta.MergeTimestamp), nil
}

// IsRangeDescriptorKey returns nil if the key decodes as a RangeDescriptor.
func IsRangeDescriptorKey(key storage.MVCCKey) error {
	_, suffix, _, err := keys.DecodeRangeKey(key.Key)
	if err != nil {
		return err
	}
	if !bytes.Equal(suffix, keys.LocalRangeDescriptorSuffix) {
		return fmt.Errorf("wrong suffix: %s", suffix)
	}
	return nil
}

func getProtoValue(data []byte, msg protoutil.Message) error {
	value := roachpb.Value{
		RawBytes: data,
	}
	return value.GetProto(msg)
}

func descStr(desc roachpb.RangeDescriptor) string {
	return fmt.Sprintf("[%s, %s)\n\tRaw:%s\n",
		desc.StartKey, desc.EndKey, &desc)
}

func maybeUnmarshalInline(v []byte, dest protoutil.Message) error {
	var meta enginepb.MVCCMetadata
	if err := protoutil.Unmarshal(v, &meta); err != nil {
		return err
	}
	value := roachpb.Value{
		RawBytes: meta.RawBytes,
	}
	return value.GetProto(dest)
}

type stringifyWriteBatch kvserverpb.WriteBatch

func (s *stringifyWriteBatch) String() string {
	wbStr, err := decodeWriteBatch((*kvserverpb.WriteBatch)(s))
	if err == nil {
		return wbStr
	}
	return fmt.Sprintf("failed to stringify write batch (%x): %s", s.Data, err)
}
