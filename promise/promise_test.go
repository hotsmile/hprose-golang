/**********************************************************\
|                                                          |
|                          hprose                          |
|                                                          |
| Official WebSite: http://www.hprose.com/                 |
|                   http://www.hprose.org/                 |
|                                                          |
\**********************************************************/
/**********************************************************\
 *                                                        *
 * promise/promise_test.go                                *
 *                                                        *
 * promise test for Go.                                   *
 *                                                        *
 * LastModified: Aug 13, 2016                             *
 * Author: Ma Bingyao <andot@hprose.com>                  *
 *                                                        *
\**********************************************************/

package promise

import "testing"

func TestNew(t *testing.T) {
	p := New()
	if p.State() != PENDING {
		t.Error("p.State must be PENDING")
	}
}

func TestResolve(t *testing.T) {
	p := Resolve(123)
	if p.State() != FULFILLED {
		t.Error("p.State must be FULFILLED")
	}
	if v, _ := p.Get(); v.(int) != 123 {
		t.Error("p.Get value be 123")
	}
}
