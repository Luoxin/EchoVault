package sorted_set

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/kelvinmwinuka/memstore/src/utils"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
)

type Plugin struct {
	name        string
	commands    []utils.Command
	categories  []string
	description string
}

func (p Plugin) Name() string {
	return p.name
}

func (p Plugin) Commands() []utils.Command {
	return p.commands
}

func (p Plugin) Description() string {
	return p.description
}

func (p Plugin) HandleCommand(ctx context.Context, cmd []string, server utils.Server, conn *net.Conn) ([]byte, error) {
	switch strings.ToLower(cmd[0]) {
	default:
		return nil, errors.New("command unknown")
	case "zadd":
		return handleZADD(ctx, cmd, server)
	case "zcard":
		return handleZCARD(ctx, cmd, server)
	case "zcount":
		return handleZCOUNT(ctx, cmd, server)
	case "zdiff":
		return handleZDIFF(ctx, cmd, server)
	case "zdiffstore":
		return handleZDIFFSTORE(ctx, cmd, server)
	case "zincrby":
		return handleZINCRBY(ctx, cmd, server)
	case "zinter":
		return handleZINTER(ctx, cmd, server)
	case "zinterstore":
		return handleZINTERSTORE(ctx, cmd, server)
	case "zmpop":
		return handleZMPOP(ctx, cmd, server)
	case "zpopmax":
		return handleZPOP(ctx, cmd, server)
	case "zpopmin":
		return handleZPOP(ctx, cmd, server)
	case "zmscore":
		return handleZMSCORE(ctx, cmd, server)
	case "zscore":
		return handleZSCORE(ctx, cmd, server)
	case "zrank":
		return handleZRANK(ctx, cmd, server)
	case "zrevrank":
		return handleZRANK(ctx, cmd, server)
	case "zrem":
		return handleZREM(ctx, cmd, server)
	case "zrandmember":
		return handleZRANDMEMBER(ctx, cmd, server)
	case "zremrangebylex":
		return handleZREMRANGEBYLEX(ctx, cmd, server)
	case "zremrangebyscore":
		return handleZREMRANGEBYSCORE(ctx, cmd, server)
	case "zremrangebyrank":
		return handleZREMRANGEBYRANK(ctx, cmd, server)
	case "zrange":
		return handleZRANGE(ctx, cmd, server)
	case "zrangestore":
		return handleZRANGESTORE(ctx, cmd, server)
	case "zunion":
		return handleZUNION(ctx, cmd, server)
	case "zunionstore":
		return handleZUNIONSTORE(ctx, cmd, server)
	}
}

func handleZADD(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]

	var updatePolicy interface{} = nil
	var comparison interface{} = nil
	var changed interface{} = nil
	var incr interface{} = nil

	// Find the first valid score and this will be the start of the score/member pairs
	var membersStartIndex int
	for i := 0; i < len(cmd); i++ {
		if membersStartIndex != 0 {
			break
		}
		switch utils.AdaptType(cmd[i]).(type) {
		case string:
			if utils.Contains([]string{"-inf", "+inf"}, strings.ToLower(cmd[i])) {
				membersStartIndex = i
			}
		case float64:
			membersStartIndex = i
		case int:
			membersStartIndex = i
		}
	}

	if membersStartIndex < 2 || len(cmd[membersStartIndex:])%2 != 0 {
		return nil, errors.New("score/member pairs must be float/string")
	}

	var members []MemberParam

	for i := 0; i < len(cmd[membersStartIndex:]); i++ {
		if i%2 != 0 {
			continue
		}
		score := utils.AdaptType(cmd[membersStartIndex:][i])
		switch score.(type) {
		default:
			return nil, errors.New("invalid score in score/member list")
		case string:
			var s float64
			if strings.ToLower(score.(string)) == "-inf" {
				s = math.Inf(-1)
				members = append(members, MemberParam{
					value: Value(cmd[membersStartIndex:][i+1]),
					score: Score(s),
				})
			}
			if strings.ToLower(score.(string)) == "+inf" {
				s = math.Inf(1)
				members = append(members, MemberParam{
					value: Value(cmd[membersStartIndex:][i+1]),
					score: Score(s),
				})
			}
		case float64:
			s, _ := score.(float64)
			members = append(members, MemberParam{
				value: Value(cmd[membersStartIndex:][i+1]),
				score: Score(s),
			})
		case int:
			s, _ := score.(int)
			members = append(members, MemberParam{
				value: Value(cmd[membersStartIndex:][i+1]),
				score: Score(s),
			})
		}
	}

	// Parse options using membersStartIndex as the upper limit
	if membersStartIndex > 2 {
		options := cmd[2:membersStartIndex]
		for _, option := range options {
			if utils.Contains([]string{"xx", "nx"}, strings.ToLower(option)) {
				updatePolicy = option
				continue
			}
			if utils.Contains([]string{"gt", "lt"}, strings.ToLower(option)) {
				comparison = option
				continue
			}
			if strings.EqualFold(option, "ch") {
				changed = option
				continue
			}
			if strings.EqualFold(option, "incr") {
				incr = option
				continue
			}
			return nil, fmt.Errorf("invalid option %s", option)
		}
	}

	if server.KeyExists(key) {
		// Key exists
		_, err := server.KeyLock(ctx, key)
		if err != nil {
			return nil, err
		}
		defer server.KeyUnlock(key)
		set, ok := server.GetValue(key).(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set")
		}
		count, err := set.AddOrUpdate(members, updatePolicy, comparison, changed, incr)
		if err != nil {
			return nil, err
		}
		// If INCR option is provided, return the new score value
		if incr != nil {
			m := set.Get(members[0].value)
			return []byte(fmt.Sprintf("+%f\r\n\r\n", m.score)), nil
		}

		return []byte(fmt.Sprintf(":%d\r\n\r\n", count)), nil
	}

	// Key does not exist
	_, err := server.CreateKeyAndLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set := NewSortedSet(members)
	server.SetValue(ctx, key, set)

	return []byte(fmt.Sprintf(":%d\r\n\r\n", set.Cardinality())), nil
}

func handleZCARD(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 2 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}
	key := cmd[1]

	if !server.KeyExists(key) {
		return []byte("*0\r\n\r\n"), nil
	}

	_, err := server.KeyRLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	return []byte(fmt.Sprintf(":%d\r\n\r\n", set.Cardinality())), nil
}

func handleZCOUNT(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]

	minimum := Score(math.Inf(-1))
	switch utils.AdaptType(cmd[2]).(type) {
	default:
		return nil, errors.New("min constraint must be a double")
	case string:
		if strings.ToLower(cmd[2]) == "+inf" {
			minimum = Score(math.Inf(1))
		} else {
			return nil, errors.New("min constraint must be a double")
		}
	case float64:
		s, _ := utils.AdaptType(cmd[2]).(float64)
		minimum = Score(s)
	case int:
		s, _ := utils.AdaptType(cmd[2]).(int)
		minimum = Score(s)
	}

	maximum := Score(math.Inf(1))
	switch utils.AdaptType(cmd[3]).(type) {
	default:
		return nil, errors.New("max constraint must be a double")
	case string:
		if strings.ToLower(cmd[3]) == "-inf" {
			maximum = Score(math.Inf(-1))
		} else {
			return nil, errors.New("max constraint must be a double")
		}
	case float64:
		s, _ := utils.AdaptType(cmd[3]).(float64)
		maximum = Score(s)
	case int:
		s, _ := utils.AdaptType(cmd[3]).(int)
		maximum = Score(s)
	}

	if !server.KeyExists(key) {
		return []byte("*0\r\n\r\n"), nil
	}

	_, err := server.KeyRLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	var members []MemberParam
	for _, m := range set.GetAll() {
		if m.score >= minimum && m.score <= maximum {
			members = append(members, m)
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n\r\n", len(members))), nil
}

func handleZDIFF(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	keys := utils.Filter(cmd[1:], func(s string) bool {
		return !strings.EqualFold(s, "withscores")
	})

	withscoresIndex := slices.IndexFunc(cmd, func(s string) bool {
		return strings.EqualFold(s, "withscores")
	})
	if withscoresIndex > -1 && withscoresIndex < 2 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(key)
			}
		}
	}()

	var sets []*SortedSet

	for _, key := range keys {
		if !server.KeyExists(key) {
			continue
		}
		locked, err := server.KeyRLock(ctx, key)
		if err != nil {
			return nil, err
		}
		locks[key] = locked
		set, ok := server.GetValue(key).(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at error %s is not a sorted set", key)
		}
		sets = append(sets, set)
	}

	var diff *SortedSet

	switch len(sets) {
	case 0:
		return []byte("*0\r\n\r\n"), nil
	case 1:
		diff = sets[0]
	default:
		diff = sets[0].Subtract(sets[1:])
	}

	res := fmt.Sprintf("*%d", diff.Cardinality())
	includeScores := withscoresIndex != -1 && withscoresIndex >= 2

	var str string
	for i, m := range diff.GetAll() {
		if includeScores {
			str = fmt.Sprintf("%s %f", m.value, m.score)
			res += fmt.Sprintf("\r\n$%d\r\n%s", len(str), str)
		} else {
			str = string(m.value)
			res += fmt.Sprintf("\r\n$%d\r\n%s", len(str), str)
		}
		if i == diff.Cardinality()-1 {
			res += "\r\n\r\n"
		}
	}

	return []byte(res), nil
}

func handleZDIFFSTORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	destination := cmd[1]
	keys := cmd[2:]

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(key)
			}
		}
	}()

	var sets []*SortedSet

	for _, key := range keys {
		if server.KeyExists(key) {
			_, err := server.KeyRLock(ctx, key)
			if err != nil {
				return nil, err
			}
			set, ok := server.GetValue(key).(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", key)
			}
			sets = append(sets, set)
		}
	}

	var diff *SortedSet

	if len(sets) > 1 {
		diff = sets[0].Subtract(sets[1:])
	} else if len(sets) == 1 {
		diff = sets[0]
	} else {
		return nil, errors.New("not enough sorted sets to calculate difference")
	}

	if server.KeyExists(destination) {
		_, err := server.KeyLock(ctx, destination)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := server.CreateKeyAndLock(ctx, destination)
		if err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(destination)

	server.SetValue(ctx, destination, diff)

	return []byte(fmt.Sprintf(":%d\r\n\r\n", diff.Cardinality())), nil
}

func handleZINCRBY(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]
	member := Value(cmd[3])
	var increment Score

	switch utils.AdaptType(cmd[2]).(type) {
	default:
		return nil, errors.New("increment must be a double")
	case string:
		if strings.EqualFold("-inf", strings.ToLower(cmd[2])) {
			increment = Score(math.Inf(-1))
		} else if strings.EqualFold("+inf", strings.ToLower(cmd[2])) {
			increment = Score(math.Inf(1))
		} else {
			return nil, errors.New("increment must be a double")
		}
	case float64:
		s, _ := utils.AdaptType(cmd[2]).(float64)
		increment = Score(s)
	case int:
		s, _ := utils.AdaptType(cmd[2]).(int)
		increment = Score(s)
	}

	if server.KeyExists(key) {
		_, err := server.KeyLock(ctx, key)
		if err != nil {
			return nil, err
		}
		defer server.KeyUnlock(key)
		set, ok := server.GetValue(key).(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", key)
		}
		_, err = set.AddOrUpdate(
			[]MemberParam{{value: member, score: increment}}, "xx", nil, nil, "incr")
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("+%f\r\n\r\n", set.Get(member).score)), nil
	}

	_, err := server.CreateKeyAndLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set := NewSortedSet([]MemberParam{
		{
			value: member,
			score: increment,
		},
	})
	server.SetValue(ctx, key, set)

	return []byte(fmt.Sprintf("+%f\r\n\r\n", set.Get(member).score)), nil
}

func handleZINTER(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 2 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	keys, weights, aggregate, withscores, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(key)
			}
		}
	}()

	var sets []*SortedSet

	for _, key := range keys {
		if server.KeyExists(key) {
			_, err := server.KeyRLock(ctx, key)
			if err != nil {
				return nil, err
			}
			locks[key] = true
			set, ok := server.GetValue(key).(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", key)
			}
			sets = append(sets, set)
		}
	}

	var intersect *SortedSet

	if len(sets) > 1 {
		if intersect, err = sets[0].Intersect(sets[1:], weights, aggregate); err != nil {
			return nil, err
		}
	} else if len(sets) == 1 {
		intersect = sets[0]
	} else {
		return nil, errors.New("not enough sets to form an intersect")
	}

	res := fmt.Sprintf("*%d", intersect.Cardinality())

	if intersect.Cardinality() > 0 {
		for i, m := range intersect.GetAll() {
			if withscores {
				s := fmt.Sprintf("%s %f", m.value, m.score)
				res += fmt.Sprintf("\r\n$%d\r\n%s", len(s), s)
			} else {
				res += fmt.Sprintf("\r\n%s", m.value)
			}
			if i == intersect.Cardinality()-1 {
				res += "\r\n\r\n"
			}
		}
	} else {
		res += "\r\n\r\n"
	}

	return []byte(res), nil
}

func handleZINTERSTORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	destination := cmd[1]

	cmd = slices.DeleteFunc(cmd, func(s string) bool {
		return s == destination
	})

	keys, weights, aggregate, _, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(key)
			}
		}
	}()

	var sets []*SortedSet

	for _, key := range keys {
		_, err := server.KeyRLock(ctx, key)
		if err != nil {
			return nil, err
		}
		locks[key] = true
		set, ok := server.GetValue(key).(*SortedSet)
		if !ok {
			return nil, fmt.Errorf("value at %s is not a sorted set", key)
		}
		sets = append(sets, set)
	}

	var intersect *SortedSet

	if len(sets) > 1 {
		if intersect, err = sets[0].Intersect(sets[1:], weights, aggregate); err != nil {
			return nil, err
		}
	} else if len(sets) == 1 {
		intersect = sets[0]
	} else {
		return nil, errors.New("not enough sets to form an intersect")
	}

	if server.KeyExists(destination) {
		if _, err := server.KeyLock(ctx, destination); err != nil {
			return nil, err
		}
	} else {
		if _, err := server.CreateKeyAndLock(ctx, destination); err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(destination)

	server.SetValue(ctx, destination, intersect)

	return []byte(fmt.Sprintf(":%d\r\n\r\n", intersect.Cardinality())), nil
}

func handleZMPOP(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 2 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	count := 1
	policy := "min"
	modifierIdx := -1

	// Parse COUNT from command
	countIdx := slices.IndexFunc(cmd, func(s string) bool {
		return strings.ToLower(s) == "count"
	})
	if countIdx != -1 {
		if countIdx < 2 {
			return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
		}
		if countIdx == len(cmd)-1 {
			return nil, errors.New("count must be a positive integer")
		}
		c, err := strconv.Atoi(cmd[countIdx+1])
		if err != nil {
			return nil, err
		}
		if c <= 0 {
			return nil, errors.New("count must be a positive integer")
		}
		count = c
		modifierIdx = countIdx
	}

	// Parse MIN/MAX from the command
	policyIdx := slices.IndexFunc(cmd, func(s string) bool {
		return slices.Contains([]string{"min", "max"}, strings.ToLower(s))
	})
	if policyIdx != -1 {
		if policyIdx < 2 {
			return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
		}
		policy = strings.ToLower(cmd[policyIdx])
		if modifierIdx == -1 || (policyIdx < modifierIdx) {
			modifierIdx = policyIdx
		}
	}

	var keys []string
	if modifierIdx == -1 {
		keys = cmd[1:]
	} else {
		keys = cmd[1:modifierIdx]
	}

	for _, key := range keys {
		if server.KeyExists(key) {
			_, err := server.KeyLock(ctx, key)
			if err != nil {
				continue
			}
			v, ok := server.GetValue(key).(*SortedSet)
			if !ok || v.Cardinality() == 0 {
				server.KeyUnlock(key)
				continue
			}
			popped, err := v.Pop(count, policy)
			if err != nil {
				server.KeyUnlock(key)
				return nil, err
			}
			server.KeyUnlock(key)
			if popped.Cardinality() == 0 {
				return []byte("+(nil)\r\n\r\n"), nil
			}

			res := fmt.Sprintf("*%d", popped.Cardinality())
			for i, m := range popped.GetAll() {
				s := fmt.Sprintf("%s %f", m.value, m.score)
				res += fmt.Sprintf("\r\n$%d\r\n%s", len(s), s)
				if i == popped.Cardinality()-1 {
					res += "\r\n\r\n"
				}
			}

			return []byte(res), nil
		}
	}

	return []byte("+(nil)\r\n\r\n"), nil
}

func handleZPOP(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 2 || len(cmd) > 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]
	count := 1
	policy := "min"

	if strings.EqualFold(cmd[0], "zpopmax") {
		policy = "max"
	}

	if len(cmd) == 3 {
		c, err := strconv.Atoi(cmd[2])
		if err != nil {
			return nil, err
		}
		count = c
	}

	if !server.KeyExists(key) {
		return []byte("+(nil)\r\n\r\n"), nil
	}

	_, err := server.KeyLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at key %s is not a sorted set", key)
	}

	popped, err := set.Pop(count, policy)
	if err != nil {
		return nil, err
	}

	res := fmt.Sprintf("*%d", popped.Cardinality())
	for i, m := range popped.GetAll() {
		s := fmt.Sprintf("%s %f", m.value, m.score)
		res += fmt.Sprintf("\r\n$%d\r\n%s", len(s), s)
		if i == popped.Cardinality()-1 {
			res += "\r\n\r\n"
		}
	}

	return []byte(res), nil
}

func handleZMSCORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]

	if !server.KeyExists(key) {
		return []byte("*0\r\n\r\n"), nil
	}

	_, err := server.KeyRLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	res := fmt.Sprintf("*%d", len(cmd[2:]))
	var member MemberObject
	for i, m := range cmd[2:] {
		member = set.Get(Value(m))
		if !member.exists {
			res = fmt.Sprintf("%s\r\n+(nil)", res)
		} else {
			res = fmt.Sprintf("%s\r\n+%f", res, member.score)
		}
		if i == len(cmd[2:])-1 {
			res += "\r\n\r\n"
		}
	}

	return []byte(res), nil
}

func handleZRANDMEMBER(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	return nil, errors.New("ZRANDMEMBER not implemented")
}

func handleZRANK(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 || len(cmd) > 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]
	member := cmd[2]
	withscores := false

	if len(cmd) == 4 && strings.EqualFold(cmd[3], "withscores") {
		withscores = true
	}

	if !server.KeyExists(key) {
		return []byte("_\r\n\r\n"), nil
	}

	if _, err := server.KeyRLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()
	slices.SortFunc(members, func(a, b MemberParam) int {
		if strings.EqualFold(cmd[0], "zrevrank") {
			return cmp.Compare(b.score, a.score)
		}
		return cmp.Compare(a.score, b.score)
	})

	for i := 0; i < len(members); i++ {
		if members[i].value == Value(member) {
			if withscores {
				score := strconv.FormatFloat(float64(members[i].score), 'f', -1, 64)
				return []byte(fmt.Sprintf("*2\r\n:%d\r\n$%d\r\n%s\r\n\r\n", i, len(score), score)), nil
			} else {
				return []byte(fmt.Sprintf(":%d\r\n\r\n", i)), nil
			}
		}
	}

	return []byte("_\r\n\r\n"), nil
}

func handleZREM(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]

	if !server.KeyExists(key) {
		return []byte(":0\r\n\r\n"), nil
	}

	_, err := server.KeyLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	deletedCount := 0
	for _, m := range cmd[2:] {
		if set.Remove(Value(m)) {
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n\r\n", deletedCount)), nil
}

func handleZSCORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}
	key := cmd[1]
	if !server.KeyExists(key) {
		return []byte("+(nil)\r\n\r\n"), nil
	}
	_, err := server.KeyRLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyRUnlock(key)
	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}
	member := set.Get(Value(cmd[2]))
	if !member.exists {
		return []byte("+(nil)\r\n\r\n"), nil
	}
	return []byte(fmt.Sprintf("+%f\r\n\r\n", member.score)), nil
}

func handleZREMRANGEBYSCORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	deletedCount := 0
	key := cmd[1]

	minimum, err := strconv.ParseFloat(cmd[2], 64)
	if err != nil {
		return nil, err
	}

	maximum, err := strconv.ParseFloat(cmd[3], 64)
	if err != nil {
		return nil, err
	}

	if !server.KeyExists(key) {
		return []byte("_\r\n\r\n"), nil
	}

	if _, err := server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	for _, m := range set.GetAll() {
		if m.score >= Score(minimum) && m.score <= Score(maximum) {
			set.Remove(m.value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n\r\n", deletedCount)), nil
}

func handleZREMRANGEBYRANK(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]

	start, err := strconv.Atoi(cmd[2])
	if err != nil {
		return nil, err
	}

	stop, err := strconv.Atoi(cmd[3])
	if err != nil {
		return nil, err
	}

	if !server.KeyExists(key) {
		return []byte("_\r\n\r\n"), nil
	}

	if _, err := server.KeyLock(ctx, key); err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	if start < 0 {
		start = start + set.Cardinality()
	}
	if stop < 0 {
		stop = stop + set.Cardinality()
	}

	if start < 0 || start > set.Cardinality()-1 || stop < 0 || start > set.Cardinality()-1 {
		return nil, errors.New("indices out of bounds")
	}

	members := set.GetAll()
	slices.SortFunc(members, func(a, b MemberParam) int {
		return cmp.Compare(a.score, b.score)
	})

	deletedCount := 0

	if start < stop {
		for i := start; i <= stop; i++ {
			set.Remove(members[i].value)
			deletedCount += 1
		}
	} else {
		for i := stop; i <= start; i++ {
			set.Remove(members[i].value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n\r\n", deletedCount)), nil
}

func handleZREMRANGEBYLEX(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) != 4 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	key := cmd[1]
	minimum := cmd[2]
	maximum := cmd[3]

	if !server.KeyExists(key) {
		return []byte("_\r\n\r\n"), nil
	}

	_, err := server.KeyLock(ctx, key)
	if err != nil {
		return nil, err
	}
	defer server.KeyUnlock(key)

	set, ok := server.GetValue(key).(*SortedSet)
	if !ok {
		return nil, fmt.Errorf("value at %s is not a sorted set", key)
	}

	members := set.GetAll()

	if len(members) == 0 {
		return []byte(":0\r\n\r\n"), nil
	}
	if len(members) == 1 {
		// Remove the single member if it's within the range
		if slices.Contains([]int{1, 0}, compareLex(string(members[0].value), minimum)) &&
			slices.Contains([]int{-1, 0}, compareLex(string(members[0].value), maximum)) {
			set.Remove(members[0].value)
			return []byte(":1\r\n\r\n"), nil
		}
	}

	// Check if all the members have the same score. If not, return nil
	for i := 0; i < len(members)-1; i++ {
		if members[i].score != members[i+1].score {
			return []byte("_\r\n\r\n"), nil
		}
	}

	deletedCount := 0

	// All the members have the same score
	for _, m := range members {
		if slices.Contains([]int{1, 0}, compareLex(string(m.value), minimum)) &&
			slices.Contains([]int{-1, 0}, compareLex(string(m.value), maximum)) {
			set.Remove(m.value)
			deletedCount += 1
		}
	}

	return []byte(fmt.Sprintf(":%d\r\n\r\n", deletedCount)), nil
}

func handleZRANGE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	return nil, errors.New("ZRANGE not implemented")
}

func handleZRANGESTORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	return nil, errors.New("ZRANGESTORE not implemented")
}

func handleZUNION(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 2 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	keys, weights, aggregate, withscores, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(key)
			}
		}
	}()

	var sets []*SortedSet

	for _, key := range keys {
		if server.KeyExists(key) {
			_, err := server.KeyRLock(ctx, key)
			if err != nil {
				return nil, err
			}
			locks[key] = true
			set, ok := server.GetValue(key).(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at key %s is not a sorted set", key)
			}
			sets = append(sets, set)
		}
	}

	var union *SortedSet

	if len(sets) > 1 {
		union, err = sets[0].Union(sets[1:], weights, aggregate)
		if err != nil {
			return nil, err
		}
	} else if len(sets) == 1 {
		union = sets[0]
	} else {
		return nil, errors.New("no sorted sets to form union")
	}

	res := fmt.Sprintf("*%d", union.Cardinality())
	for i, m := range union.GetAll() {
		if withscores {
			s := fmt.Sprintf("%s %f", m.value, m.score)
			res += fmt.Sprintf("\r\n$%d\r\n%s", len(s), s)
		} else {
			res += fmt.Sprintf("\r\n+%s", m.value)
		}
		if i == union.Cardinality()-1 {
			res += "\r\n\r\n"
		}
	}

	return []byte(res), nil
}

func handleZUNIONSTORE(ctx context.Context, cmd []string, server utils.Server) ([]byte, error) {
	if len(cmd) < 3 {
		return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
	}

	destination := cmd[1]

	// Remove destination key from list of keys
	cmd = slices.DeleteFunc(cmd, func(s string) bool {
		return s == destination
	})

	keys, weights, aggregate, _, err := extractKeysWeightsAggregateWithScores(cmd)
	if err != nil {
		return nil, err
	}

	locks := make(map[string]bool)
	defer func() {
		for key, locked := range locks {
			if locked {
				server.KeyRUnlock(key)
			}
		}
	}()

	var sets []*SortedSet

	for _, key := range keys {
		if server.KeyExists(key) {
			_, err := server.KeyRLock(ctx, key)
			if err != nil {
				return nil, err
			}
			locks[key] = true
			set, ok := server.GetValue(key).(*SortedSet)
			if !ok {
				return nil, fmt.Errorf("value at %s is not a sorted set", key)
			}
			sets = append(sets, set)
		}
	}

	var union *SortedSet

	if len(sets) > 1 {
		union, err = sets[0].Union(sets[1:], weights, aggregate)
		if err != nil {
			return nil, err
		}
	} else if len(sets) == 1 {
		union = sets[0]
	} else {
		return nil, errors.New("no sorted sets to form union")
	}

	if server.KeyExists(destination) {
		if _, err := server.KeyLock(ctx, destination); err != nil {
			return nil, err
		}
	} else {
		if _, err := server.CreateKeyAndLock(ctx, destination); err != nil {
			return nil, err
		}
	}
	defer server.KeyUnlock(destination)

	server.SetValue(ctx, destination, union)

	return []byte(fmt.Sprintf(":%d\r\n\r\n", union.Cardinality())), nil
}

func NewModule() Plugin {
	return Plugin{
		name: "SortedSetCommand",
		commands: []utils.Command{
			{
				Command:    "zadd",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZADD key [NX | XX] [GT | LT] [CH] [INCR] score member [score member...])
Adds all the specified members with the specified scores to the sorted set at the key`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 4 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:     "zcard",
				Categories:  []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZCARD key) Returns the set cardinality of the sorted set at key.`,
				Sync:        false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:], nil
				},
			},
			{
				Command:    "zcount",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZCOUNT key min max) 
Returns the number of elements in the sorted set key with scores in the range of min and max.`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 4 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:    "zdiff",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZDIFF key [key...] [WITHSCORES]) 
Computes the difference between all the sorted sets specifies in the list of keys and returns the result.`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					keys := utils.Filter(cmd[1:], func(elem string) bool {
						return !strings.EqualFold(elem, "WITHSCORES")
					})
					return keys, nil
				},
			},
			{
				Command:    "zdiffstore",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZDIFFSTORE destination key [key...]). 
Computes the difference between all the sorted sets specifies in the list of keys. Stores the result in destination.`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[2:], nil
				},
			},
			{
				Command:    "zincrby",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZINCRBY key increment member). 
Increments the score of the specified sorted set's member by the increment. If the member does not exist, it is created.
If the key does not exist, it is created with new sorted set and the member added with the increment as its score.`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 4 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:    "zinter",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZINTER key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]).
Computes the intersection of the sets in the keys, with weights, aggregate and scores`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					endIdx := slices.IndexFunc(cmd[1:], func(s string) bool {
						if strings.EqualFold(s, "WEIGHTS") ||
							strings.EqualFold(s, "AGGREGATE") ||
							strings.EqualFold(s, "WITHSCORES") {
							return true
						}
						return false
					})
					if endIdx == -1 {
						return cmd[1:], nil
					}
					if endIdx >= 1 {
						return cmd[1 : endIdx+1], nil
					}
					// TODO: Change this to syntax error
					return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
				},
			},
			{
				Command:    "zinterstore",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `
(ZINTERSTORE destination key [key ...] [WEIGHTS weight [weight ...]] [AGGREGATE <SUM | MIN | MAX>] [WITHSCORES]).
Computes the intersection of the sets in the keys, with weights, aggregate and scores. The result is stored in destination.`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					endIdx := slices.IndexFunc(cmd[1:], func(s string) bool {
						if strings.EqualFold(s, "WEIGHTS") ||
							strings.EqualFold(s, "AGGREGATE") ||
							strings.EqualFold(s, "WITHSCORES") {
							return true
						}
						return false
					})
					if endIdx == -1 {
						return cmd[1:], nil
					}
					if endIdx >= 2 {
						return cmd[1:endIdx], nil
					}
					return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
				},
			},
			{
				Command:    "zmpop",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZMPOP key [key ...] <MIN | MAX> [COUNT count])
Pop a 'count' elements from sorted set. MIN or MAX determines whether to pop elements with the lowest or highest scores
respectively.`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					endIdx := slices.IndexFunc(cmd, func(s string) bool {
						return utils.Contains([]string{"MIN", "MAX", "COUNT"}, strings.ToUpper(s))
					})
					if endIdx == -1 {
						return cmd[1:], nil
					}
					if endIdx >= 2 {
						return cmd[1:endIdx], nil
					}
					return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
				},
			},
			{
				Command:    "zmscore",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZMSCORE key member [member ...])
Returns the associated scores of the specified member in the sorted set. 
Returns nil for members that do not exist in the set`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:    "zpopmax",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZPOPMAX key [count])
Removes and returns 'count' number of members in the sorted set with the highest scores. Default count is 1.`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:    "zpopmin",
				Categories: []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZPOPMIN key [count])
Removes and returns 'count' number of members in the sorted set with the lowest scores. Default count is 1.`,
				Sync: true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return []string{cmd[1]}, nil
				},
			},
			{
				Command:    "zrandmember",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZRANDMEMBER key [count [WITHSCORES]])
Return a list of length equivalent to count containing random members of the sorted set.
If count is negative, repeated elements are allowed. If count is positive, the returned elements will be distinct.
WITHSCORES modifies the result to include scores in the result.`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 2 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:    "zrank",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZRANK key member [WITHSCORE])
Returns the rank of the specified member in the sorted set. WITHSCORE modifies the result to also return the score.`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:     "zrem",
				Categories:  []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: `(ZREM key member [member ...]) Removes the listed members from the sorted set.`,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:    "zrevrank",
				Categories: []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: `(ZREVRANK key member [WITHSCORE])
Returns the rank of the member in the sorted set. WITHSCORE modifies the result to include the score.`,
				Sync: false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) < 3 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:     "zscore",
				Categories:  []string{utils.SortedSetCategory, utils.ReadCategory, utils.FastCategory},
				Description: `(ZSCORE key member) Returns the score of the member in the sorted set.`,
				Sync:        false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					if len(cmd) != 3 {
						return nil, errors.New(utils.WRONG_ARGS_RESPONSE)
					}
					return cmd[1:2], nil
				},
			},
			{
				Command:     "zremrangebylex",
				Categories:  []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zremrangebyrank",
				Categories:  []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zremrangebyscore",
				Categories:  []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zlexcount",
				Categories:  []string{},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zrange",
				Categories:  []string{},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zrangebylex",
				Categories:  []string{},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zrangebyscore",
				Categories:  []string{},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zrangestore",
				Categories:  []string{},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zunion",
				Categories:  []string{utils.SortedSetCategory, utils.ReadCategory},
				Description: ``,
				Sync:        false,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
			{
				Command:     "zunionstore",
				Categories:  []string{utils.SortedSetCategory, utils.WriteCategory},
				Description: ``,
				Sync:        true,
				KeyExtractionFunc: func(cmd []string) ([]string, error) {
					return []string{}, nil
				},
			},
		},
		description: "Handle commands on sorted set data type",
	}
}
