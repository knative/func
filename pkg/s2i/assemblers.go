package s2i

import (
	"fmt"

	fn "knative.dev/func/pkg/functions"
)

/*
Assemble Scripts Patched for Functions Support

These are minimally patched assemblers taken from the S2I builders.   For
example the Python assemble script can be extracted with:

docker run --rm registry.access.redhat.com/ubi8/python-39 \
  cat /usr/libexec/s2i/assemble > bin/assemble

The scripts are modified slightly to support Functions as explained by
their individual constants below.

While there is unlikely to be incompatibilities going forward, this is still
a support burden as these scripts should be updated periodically, at least
once per major version of their underlying toolchain.

Once Functions is stable and post 1.0, it would be worth exploring adding
Functions support in the upstream base scripts themselves via environment
variable.
*/

func assembler(f fn.Function) (string, error) {
	switch f.Runtime {
	case "go":
		return GoAssembler, nil
	case "python":
		return PythonAssembler, nil
	default:
		return "", fmt.Errorf("no assembler defined for runtime %q", f.Runtime)
	}
}

// GoAssembler
//
// Adapted from /usr/libexec/s2i/assemble within the UBI-8 go-toolchain
// such that the "go build" command builds subdirectory .s2i/builds/last
// (where main resides) rather than the root.
const GoAssembler = `
#!/bin/bash
set -e
pushd /tmp/src
if [[ $(go list -f {{.Incomplete}}) == "true" ]]; then
    INSTALL_URL=${INSTALL_URL:-$IMPORT_URL}
    if [[ ! -z "$IMPORT_URL" ]]; then
        popd
        echo "Assembling GOPATH"
        export GOPATH=$(realpath $HOME/go)
	    mkdir -p $GOPATH/src/$IMPORT_URL mv /tmp/src/* $GOPATH/src/$IMPORT_URL
        if [[ -d /tmp/artifacts/pkg ]]; then
            echo "Restoring previous build artifacts"
            mv /tmp/artifacts/pkg $GOPATH
        fi
        # Resolve dependencies, ignore if vendor present
        if [[ ! -d $GOPATH/src/$INSTALL_URL/vendor ]]; then
            echo "Resolving dependencies"
            pushd $GOPATH/src/$INSTALL_URL
            go get
            popd
        fi
        # lets build
        pushd $GOPATH/src/$INSTALL_URL
        echo "Building"
        go install -i $INSTALL_URL
        mv $GOPATH/bin/* /opt/app-root/gobinary
        popd
        exit
    fi
    /$STI_SCRIPTS_PATH/usage
    exit 1
else
    pushd .s2i/builds/last
    go mod tidy
    go build -o /opt/app-root/gobinary
    popd
    popd
fi
`

// PythonAssembler
//
// Adapted from /usr/libexec/s2i/assemble within the UBI-8 python-toolchain
// such that the the script executes from subdirectory .s2i/builds/last
// (where main resides) rather than the root, and indicates the main is
// likewise in .s2i/builds/last/service/main.py via Procfile.  See the comment
// inline on line 50 of the script for where the directory change instruction
// was added.
const PythonAssembler = `
#!/bin/bash

function is_django_installed() {
  python -c "import django" &>/dev/null
}

function should_collectstatic() {
  is_django_installed && [[ -z "$DISABLE_COLLECTSTATIC" ]]
}

function virtualenv_bin() {
    # New versions of Python (>3.6) should use venv module
    # from stdlib instead of virtualenv package
    python3.9 -m venv $1
}

# Install pipenv or micropipenv to the separate virtualenv to isolate it
# from system Python packages and packages in the main
# virtualenv. Executable is simlinked into ~/.local/bin
# to be accessible. This approach is inspired by pipsi
# (pip script installer).
function install_tool() {
  echo "---> Installing $1 packaging tool ..."
  VENV_DIR=$HOME/.local/venvs/$1
  virtualenv_bin "$VENV_DIR"
  # First, try to install the tool without --isolated which means that if you
  # have your own PyPI mirror, it will take it from there. If this try fails, try it
  # again with --isolated which ignores external pip settings (env vars, config file)
  # and installs the tool from PyPI (needs internet connetion)
  # $1$2 combines package name with [extras] or version specifier if is defined as $2
  if ! $VENV_DIR/bin/pip install -U $1$2; then
    echo "WARNING: Installation of $1 failed, trying again from official PyPI with pip --isolated install"
    $VENV_DIR/bin/pip install --isolated -U $1$2  # Combines package name with [extras] or version specifier if is defined as $2
  fi
  mkdir -p $HOME/.local/bin
  ln -s $VENV_DIR/bin/$1 $HOME/.local/bin/$1
}

set -e

# First of all, check that we don't have disallowed combination of ENVs
if [[ ! -z "$ENABLE_PIPENV" && ! -z "$ENABLE_MICROPIPENV" ]]; then
  echo "ERROR: Pipenv and micropipenv cannot be enabled at the same time!"
  # podman/buildah does not relay this exit code but it will be fixed hopefully
  # https://github.com/containers/buildah/issues/2305
  exit 3
fi

shopt -s dotglob
echo "---> Installing application source ..."
mv /tmp/src/* "$HOME"

# ---------------------------
# MODIFICATIONS FOR FUNCTIONS
echo "---> (Functions) Writing app.sh ..."
cat << 'EOF' > app.sh
#!/bin/bash
set -e
exec python .s2i/builds/last/service/main.py
EOF
chmod +x app.sh

echo "---> (Functions) Changing directory to .s2i/builds/last ..."
cd .s2i/builds/last
# END MODIFICATION FOR FUNCTIONS
# ------------------------------

# set permissions for any installed artifacts
fix-permissions /opt/app-root -P


if [[ ! -z "$UPGRADE_PIP_TO_LATEST" ]]; then
  echo "---> Upgrading pip, setuptools and wheel to latest version ..."
  if ! pip install -U pip setuptools wheel; then
    echo "WARNING: Installation of the latest pip, setuptools and wheel failed, trying again from official PyPI with pip --isolated install"
    pip install --isolated -U pip setuptools wheel
  fi
fi

if [[ ! -z "$ENABLE_PIPENV" ]]; then
  if [[ ! -z "$PIN_PIPENV_VERSION" ]]; then
    # Add == as a prefix to pipenv version, if defined
    PIN_PIPENV_VERSION="==$PIN_PIPENV_VERSION"
  fi
  install_tool "pipenv" "$PIN_PIPENV_VERSION"
  echo "---> Installing dependencies via pipenv ..."
  if [[ -f Pipfile ]]; then
    pipenv install --deploy
  elif [[ -f requirements.txt ]]; then
    pipenv install -r requirements.txt
  fi
  # pipenv check
elif [[ ! -z "$ENABLE_MICROPIPENV" ]]; then
  install_tool "micropipenv" "[toml]"
  echo "---> Installing dependencies via micropipenv ..."
  # micropipenv detects Pipfile.lock and requirements.txt in this order
  micropipenv install --deploy
elif [[ -f requirements.txt ]]; then
  echo "---> Installing dependencies ..."
  pip install -r requirements.txt
fi

if [[ ( -f setup.py || -f setup.cfg ) && -z "$DISABLE_SETUP_PY_PROCESSING" ]]; then
  echo "---> Installing application (via setup.{py,cfg})..."
  pip install .
fi

if [[ -f pyproject.toml && -z "$DISABLE_PYPROJECT_TOML_PROCESSING" ]]; then
  echo "---> Installing application (via pyproject.toml)..."
  pip install .
fi

if should_collectstatic; then
  (
    echo "---> Collecting Django static files ..."

    APP_HOME=$(readlink -f "${APP_HOME:-.}")
    # Change the working directory to APP_HOME
    PYTHONPATH="$(pwd)${PYTHONPATH:+:$PYTHONPATH}"
    cd "$APP_HOME"

    # Look for 'manage.py' in the current directory
    manage_file=./manage.py

    if [[ ! -f "$manage_file" ]]; then
      echo "WARNING: seems that you're using Django, but we could not find a 'manage.py' file."
      echo "'manage.py collectstatic' ignored."
      exit
    fi

    if ! python $manage_file collectstatic --dry-run --noinput &> /dev/null; then
      echo "WARNING: could not run 'manage.py collectstatic'. To debug, run:"
      echo "    $ python $manage_file collectstatic --noinput"
      echo "Ignore this warning if you're not serving static files with Django."
      exit
    fi

    python $manage_file collectstatic --noinput
  )
fi

# set permissions for any installed artifacts
fix-permissions /opt/app-root -P
`
