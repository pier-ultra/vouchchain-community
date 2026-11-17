package approval

import "testing"

func TestConditionMatch(t *testing.T) {
	ctx := map[string]any{"amount": 250000.0, "counterParty": "华东供应商", "status": "draft", "riskLevel": "high"}
	cases := []struct {
		c    Condition
		want bool
	}{
		{Condition{Field: "amount", Op: "gt", Value: "100000"}, true},
		{Condition{Field: "amount", Op: "lte", Value: "100000"}, false},
		{Condition{Field: "counterParty", Op: "contains", Value: "华东"}, true},
		{Condition{Field: "status", Op: "eq", Value: "draft"}, true},
		{Condition{Field: "riskLevel", Op: "in", Value: "high,medium"}, true},
		{Condition{Field: "riskLevel", Op: "ne", Value: "low"}, true},
		{Condition{}, true},
	}
	for i, c := range cases {
		got, err := c.c.Match(ctx)
		if err != nil || got != c.want {
			t.Fatalf("case %d: got %v err %v want %v", i, got, err, c.want)
		}
	}
	if _, err := (Condition{Field: "missing", Op: "eq", Value: "x"}).Match(ctx); err == nil {
		t.Fatal("缺少字段应报错，不能静默跳过")
	}
	if _, err := (Condition{Field: "amount", Op: "gt", Value: "不是数"}).Match(ctx); err == nil {
		t.Fatal("数值比较类型错误应报错")
	}
}

func TestSkipInactiveBranching(t *testing.T) {
	specs := []NodeSpec{
		{Name: "小额快审", Approvers: []string{"主管"}, When: &Condition{Field: "amount", Op: "lt", Value: "100000"}},
		{Name: "大额专项", Approvers: []string{"总监"}, When: &Condition{Field: "amount", Op: "gte", Value: "100000"}},
		{Name: "归档", Approvers: []string{"档案员"}},
	}
	states, seq, err := Expand(specs)
	if err != nil {
		t.Fatal(err)
	}
	// 大额：第一级被跳过，直接到第二级
	got, seq2, st, records, err := SkipInactive(states, seq, map[string]any{"amount": 250000})
	if err != nil || seq2 != 2 || st != InstanceRunning || len(records) != 1 {
		t.Fatalf("大额分支错误: seq=%d st=%s records=%v err=%v", seq2, st, records, err)
	}
	if got[0].Status != NodeSkipped {
		t.Fatal("小额节点应标 skipped")
	}
	// 小额：第二级被跳过，第一级正常
	got2, seq3, _, records2, err := SkipInactive(states, seq, map[string]any{"amount": 5000})
	if err != nil || seq3 != 1 || len(records2) != 0 || got2[0].Status != NodePending {
		t.Fatalf("小额分支错误: seq=%d err=%v", seq3, err)
	}
	// 审批后推进到第 2 级，再次跳过条件不成立的级
	got2, _, seq3, err = Apply(got2, 1, Action{Actor: "主管", Action: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	got2, seq4, _, records3, err := SkipInactive(got2, seq3, map[string]any{"amount": 5000})
	if err != nil || seq4 != 3 || len(records3) != 1 {
		t.Fatalf("推进后应跳过大额节点到归档: seq=%d records=%v err=%v", seq4, records3, err)
	}
}

func TestSkipAllApproves(t *testing.T) {
	states, seq, _ := Expand([]NodeSpec{
		{Name: "仅高风险", Approvers: []string{"法务"}, When: &Condition{Field: "riskLevel", Op: "eq", Value: "high"}},
	})
	_, _, st, records, err := SkipInactive(states, seq, map[string]any{"riskLevel": "low"})
	if err != nil || st != InstanceApproved || len(records) != 1 {
		t.Fatalf("全部节点跳过应直接通过: %s %v %v", st, records, err)
	}
}

func TestAddSignTurnsNodeIntoAndSign(t *testing.T) {
	states, seq, _ := Expand([]NodeSpec{{Name: "部门", Approvers: []string{"甲", "乙"}, Mode: ModeOr}})
	got, err := AddSign(states, seq, "甲", "丙", "and", "请财务把关")
	if err != nil || len(got) != 3 {
		t.Fatalf("加签失败: %v %v", len(got), err)
	}
	for _, s := range got {
		if s.Mode != ModeAnd {
			t.Fatal("加签后节点应改为会签")
		}
	}
	if got[2].Approver != "丙" || got[2].Origin != "addsign" {
		t.Fatalf("加签行不正确: %+v", got[2])
	}
	if _, err := AddSign(states, seq, "甲", "乙", "and", ""); err == nil {
		t.Fatal("重复加签应拒绝")
	}
	if _, err := AddSign(states, seq, "路人", "丙", "and", ""); err == nil {
		t.Fatal("非待办审批人不能加签")
	}
	// 会签：三人都要同意
	_, st, seq2, _ := Apply(got, 1, Action{Actor: "甲", Action: "approve"})
	if st != InstanceRunning || seq2 != 1 {
		t.Fatal("加签后一人通过不应结束")
	}
}

func TestTransferAndCancel(t *testing.T) {
	states, seq, _ := Expand([]NodeSpec{{Name: "部门", Approvers: []string{"甲"}}})
	got, err := Transfer(states, seq, "甲", "丁", "出差")
	if err != nil || got[0].Approver != "丁" || got[0].Origin != "transfer" {
		t.Fatalf("转交失败: %v %+v", err, got)
	}
	if _, _, _, err := Apply(got, seq, Action{Actor: "甲", Action: "approve"}); err == nil {
		t.Fatal("转交后原审批人不应还能审批")
	}
	if _, _, _, err := Apply(got, seq, Action{Actor: "丁", Action: "approve"}); err != nil {
		t.Fatal("接收人应能审批")
	}
	if _, err := Transfer(states, seq, "路人", "丁", ""); err == nil {
		t.Fatal("无待办不能转交")
	}
	if _, err := Cancel(states, ""); err == nil {
		t.Fatal("废止必须填原因")
	}
	revoked, err := Cancel(states, "合同作废重签")
	if err != nil || revoked[0].Status != NodeRevoked {
		t.Fatalf("废止失败: %v %+v", err, revoked)
	}
	multi, _, _ := Expand([]NodeSpec{{Name: "A", Approvers: []string{"甲"}}, {Name: "B", Approvers: []string{"乙"}}})
	multi[0].Status = NodeApproved
	revoked2, _ := Cancel(multi, "撤回")
	if revoked2[0].Status != NodeApproved || revoked2[1].Status != NodeRevoked {
		t.Fatal("废止应保留已审批节点、只撤待办")
	}
}
