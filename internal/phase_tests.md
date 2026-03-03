# Phase test checklist

Run:

```bash
go test ./...
```

- Phase 1: `TestPhase1_DefaultSlotsInitialized`
- Phase 2: `TestPhase2_HashRingDeterministicPrimaryAndReplicas`
- Phase 3: `TestPhase3_ReplicationFanoutAppliesToReplicas`
- Phase 4: `TestPhase4_ReadQuorumReturnsLatestAndRepairsStaleReplica`, `TestPhase4_WriteQuorumFailsIfNotEnoughAcks`
- Phase 5: `TestPhase5_ConcurrentReserveOnlyOneSucceeds`
- Phase 6: `TestPhase6_HeartbeatMarksSeenAndDetectsFailure`
- Phase 7: `TestPhase7_BullyElectionHighestIDWins`
- Phase 8: `TestPhase8_RecoveryUpdatesStaleNodeFromPeers`
- Phase 9: `TestPhase9_JoinAugmentsMembershipAndRebalances`
