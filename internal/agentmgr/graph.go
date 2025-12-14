package agentmgr

import "errors"

var ErrCycle = errors.New("dependency cycle detected")

type Adj struct {
	Out map[string][]string // node -> deps
	In  map[string][]string // reverse edges
}

func syncGraph(agents map[string]Agent) *Adj {
	g := &Adj{Out: map[string][]string{}, In: map[string][]string{}}
	for id, a := range agents {
		deps := a.Dependencies()
		g.Out[id] = append([]string(nil), deps...)
		for _, d := range deps {
			g.In[d] = append(g.In[d], id)
			if _, ok := g.Out[d]; !ok {
				g.Out[d] = nil
			}
		}
		if _, ok := g.In[id]; !ok {
			g.In[id] = nil
		}
	}
	return g
}

func TopologicalSort(agents map[string]Agent) ([]string, error) {
	graph := syncGraph(agents) // ← per your ask
	inDegree := map[string]int{}
	for n := range graph.Out {
		inDegree[n] = 0
	}
	for _, deps := range graph.Out {
		for _, d := range deps {
			inDegree[d]++
		}
	}

	var q []string
	for n, deg := range inDegree {
		if deg == 0 {
			q = append(q, n)
		}
	}

	var order []string
	for len(q) > 0 {
		n := q[len(q)-1]
		q = q[:len(q)-1]
		order = append(order, n)
		for _, child := range graph.In[n] {
			inDegree[child]--
			if inDegree[child] == 0 {
				q = append(q, child)
			}
		}
	}

	if len(order) != len(inDegree) {
		return nil, ErrCycle
	}
	return order, nil
}
