TAGS=$@
curdir=`pwd`
cd `dirname $0`

projectdir=../
cd $projectdir

# Check Dependencies
#[[ ! -f func ]] && echo "func binary not found. run 'make build' prior to run e2e." && exit 1

export BOSON_FUNC_BIN=`pwd`/func
echo Binary $BOSON_FUNC_BIN

go clean -testcache
echo go test -v -test.v -tags $TAGS ./test/e2e/
go test -v -test.v -tags $TAGS ./test/e2e/
ret=$?

cd $curdir
exit $ret
