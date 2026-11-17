package approval

import (
	"testing"
)

func mk() ([]NodeState, int) {
	specs := []NodeSpec{
		{Name: "商务部审批", Approvers: []string{"李四", "王五"}, Mode: ModeOr},
		{Name: "总经理审批", Approvers: []string{"赵总"}, Mode: ModeOr},
		{Name: "会签复核", Approvers: []string{"法务甲", "法务乙"}, Mode: ModeAnd},
	}
	states, seq, err := Expand(specs)
	if err != nil {
		panic(err)
	}
	return states, seq
}

func TestExpand(t *testing.T) {
	states, seq, err := Expand([]NodeSpec{
		{Name: "A", Approvers: []string{"a1", "a2"}, Mode: ModeAnd},
		{Name: "B", Approvers: []string{"b1"}},
	})
	if err != nil || seq != 1 || len(states) != 3 {
		t.Fatalf("expand 失败: %v %d %d", err, seq, len(states))
	}
	if _, _, err := Expand([]NodeSpec{{Name: "空", Approvers: []string{}}}); err == nil {
		t.Fatal("空审批人应报错")
	}
}

func TestOrSignAdvance(t *testing.T) {
	states, seq := mk()
	// 或签：李四通过 → 节点过，进 seq2
	states, st, seq2, err := Apply(states, seq, Action{Actor: "李四", Action: "approve"})
	if err != nil || st != InstanceRunning || seq2 != 2 {
		t.Fatalf("或签推进失败: %v %s %d", err, st, seq2)
	}
	_ = states
}

func TestAndSignRequireAll(t *testing.T) {
	states, seq := mk()
	// 直接跳到会签节点场景：先或签过前两级
	states, _, _, _ = Apply(states, seq, Action{Actor: "王五", Action: "approve"})
	states, st, seq2, _ := Apply(states, 2, Action{Actor: "赵总", Action: "approve"})
	if st != InstanceRunning || seq2 != 3 {
		t.Fatalf("应进会签节点: %s %d", st, seq2)
	}
	// 会签：第一个通过，节点未完成
	states, st, seq3, _ := Apply(states, 3, Action{Actor: "法务甲", Action: "approve"})
	if st != InstanceRunning || seq3 != 3 {
		t.Fatalf("会签未齐不应推进: %s %d", st, seq3)
	}
	// 第二个通过 → 实例 approved
	states, st, _, _ = Apply(states, 3, Action{Actor: "法务乙", Action: "approve"})
	if st != InstanceApproved {
		t.Fatalf("会签齐后应 approved: %s", st)
	}
}

func TestRejectTerminates(t *testing.T) {
	states, seq := mk()
	_, _, _, err := Apply(states, seq, Action{Actor: "李四", Action: "reject"})
	if err == nil {
		t.Fatal("驳回无意见应报错")
	}
	_, st, _, err := Apply(states, seq, Action{Actor: "王五", Action: "reject", Comment: "金额超限，不同意"})
	if err != nil || st != InstanceRejected {
		t.Fatalf("驳回应终止实例: %v %s", err, st)
	}
}

func TestWrongActorDenied(t *testing.T) {
	states, seq := mk()
	// 非当前节点审批人（赵总是 seq2）不能在 seq1 审批
	if _, _, _, err := Apply(states, seq, Action{Actor: "赵总", Action: "approve"}); err == nil {
		t.Fatal("越级审批应报错")
	}
	// 重复审批：同一人再批一次应报错
	states, _, _, _ = Apply(states, seq, Action{Actor: "李四", Action: "approve"})
	if _, _, _, err := Apply(states, 1, Action{Actor: "李四", Action: "approve"}); err == nil {
		t.Fatal("重复审批应报错")
	}
}
