package builder

import (
	"fmt"
	"gengine/context"
	"gengine/internal/base"
	"gengine/internal/core/errors"
	parser "gengine/internal/iantlr/alr"
	"gengine/internal/iparser"
	"gengine/internal/tool"
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"sort"
	"sync"
)

type RuleBuilder struct {
	Kc *base.KnowledgeContext
	Dc *context.DataContext

	buildLock sync.Mutex
}

func NewRuleBuilder(dc *context.DataContext) *RuleBuilder {
	kc := base.NewKnowledgeContext()
	return &RuleBuilder{
		Kc: kc,
		Dc: dc,
	}
}

func (builder *RuleBuilder) BuildRuleFromString(ruleString string) error {
	builder.buildLock.Lock()
	defer builder.buildLock.Unlock()

	kc := base.NewKnowledgeContext()

	in := antlr.NewInputStream(ruleString)
	lexer := parser.NewgengineLexer(in)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	listener := iparser.NewGengineParserListener(kc)

	psr := parser.NewgengineParser(stream)
	psr.BuildParseTrees = true
	//grammar listener
	errListener := iparser.NewGengineErrorListener()
	psr.AddErrorListener(errListener)
	antlr.ParseTreeWalkerDefault.Walk(listener, psr.Primary())

	if len(errListener.GrammarErrors) > 0 {
		return errors.New(fmt.Sprintf("%+v", errListener.GrammarErrors))
	}

	if len(listener.ParseErrors) > 0 {
		return errors.New(fmt.Sprintf("%+v", listener.ParseErrors))
	}

	//initial
	for _, v := range kc.RuleEntities {
		v.Initialize(builder.Dc)
	}

	//sort
	for _, v := range kc.RuleEntities {
		kc.SortRules = append(kc.SortRules, v)
	}
	if len(kc.SortRules) > 1 {
		sort.SliceStable(kc.SortRules, func(i, j int) bool {
			return kc.SortRules[i].Salience > kc.SortRules[j].Salience
		})
	}

	for k, v := range kc.SortRules {
		kc.SortRulesIndexMap[v.RuleName] = k
	}

	builder.Kc = kc
	return nil
}

//chinese comment:增量更新
// if a rule already exists, this method will use the new rule to replace the old one
// if a rule doesn't exist, this method will add the new rule to the existed rules list
// in detail: copy from old -> update the copy -> use the updated copy to replace old
func (builder *RuleBuilder) BuildRuleWithIncremental(ruleString string) error {
	//make sure incremental update is thread safety!
	builder.buildLock.Lock()
	defer builder.buildLock.Unlock()

	in := antlr.NewInputStream(ruleString)
	lexer := parser.NewgengineLexer(in)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)

	kc := base.NewKnowledgeContext()
	listener := iparser.NewGengineParserListener(kc)

	psr := parser.NewgengineParser(stream)
	psr.BuildParseTrees = true

	errListener := iparser.NewGengineErrorListener()
	psr.AddErrorListener(errListener)
	antlr.ParseTreeWalkerDefault.Walk(listener, psr.Primary())

	if len(errListener.GrammarErrors) > 0 {
		return errors.New(fmt.Sprintf("%+v", errListener.GrammarErrors))
	}

	if len(listener.ParseErrors) > 0 {
		return errors.New(fmt.Sprintf("%+v", listener.ParseErrors))
	}

	if len(kc.RuleEntities)==0 {
		return errors.New(fmt.Sprintf("no rules need to update or add."))
	}

	//copy
	newRuleEntities := make(map[string]*base.RuleEntity, len(builder.Kc.RuleEntities))
	for mk, mv :=range builder.Kc.RuleEntities{
		newRuleEntities[mk] = mv
	}

	//copy
	newSortRules := make([]*base.RuleEntity, len(builder.Kc.SortRules))
	for sk,sv:=range builder.Kc.SortRules  {
		newSortRules[sk] = sv
	}


	//kc store the new rules
	for k, v := range kc.RuleEntities {
		//init
		v.Initialize(builder.Dc)

		if vm, ok := newRuleEntities[k]; ok {
			//repalce update
			//search
			index := builder.Kc.SortRulesIndexMap[v.RuleName]
			if v.Salience == vm.Salience {
				//replace
				newSortRules[index] = v
			}else {
				newSortRules := append(newSortRules[:index], newSortRules[index+1:]...)
				//search location to insert
				low, mid := tool.BinarySearch(newSortRules, v.Salience)

				ire := []*base.RuleEntity{v}
				if mid == 0 {
					newRe := append(ire, newSortRules[low:]...)
					newSortRules = append(newSortRules[:low], newRe...)
				} else {
					newRe := append(ire, newSortRules[mid:]...)
					newSortRules = append(newSortRules[:mid], newRe...)
				}

				//update the sort index
				indexMap := make(map[string]int)
				for k, v := range newSortRules {
					indexMap[v.RuleName] = k
				}
				builder.Kc.SortRulesIndexMap = indexMap
			}

			newRuleEntities[k]= v
		}else {
			//add update
			low, mid := tool.BinarySearch(newSortRules, v.Salience)

			ire := []*base.RuleEntity{v}
			if mid == 0 {
				newRe := append(ire, newSortRules[low:]...)
				newSortRules = append(newSortRules[:low], newRe...)
			} else {
				newRe := append(ire, newSortRules[mid:]...)
				newSortRules = append(newSortRules[:mid], newRe...)
			}

			//update the sort index
			indexMap := make(map[string]int)
			for k, v := range newSortRules {
				indexMap[v.RuleName] = k
			}
			builder.Kc.SortRulesIndexMap = indexMap

			newRuleEntities[k] = v
		}
	}

	builder.Kc.RuleEntities = newRuleEntities
	builder.Kc.SortRules = newSortRules

	return nil
}
