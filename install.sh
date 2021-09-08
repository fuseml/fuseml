#!/usr/bin/env bash

# Adapted from: https://github.com/rancher/k3d/blob/main/install.sh

APP_NAME="fuseml-installer"
REPO_URL="https://github.com/fuseml/fuseml"

: "${USE_SUDO:=true}"
: "${FUSEML_INSTALLER_INSTALL_DIR:=/usr/local/bin}"
: "${VERIFY_CHECKSUM:=true}"

HAS_OPENSSL="$(type "openssl" &> /dev/null && echo true || echo false)"

# initArch discovers the architecture for this system.
initArch() {
  ARCH=$(uname -m)
  case $ARCH in
    armv5*) ARCH="armv5";;
    armv6*) ARCH="armv6";;
    armv7*) ARCH="arm";;
    aarch64) ARCH="arm64";;
    x86) ARCH="386";;
    x86_64) ARCH="amd64";;
    i686) ARCH="386";;
    i386) ARCH="386";;
  esac
}

# initOS discovers the operating system for this system.
initOS() {
  OS=$(uname|tr '[:upper:]' '[:lower:]')

  case "$OS" in
    # Minimalist GNU for Windows
    mingw*) OS='windows'; USE_SUDO="false"; FUSEML_INSTALLER_INSTALL_DIR="/usr/bin"; EXTENSION=".exe";;
  esac
}

# runs the given command as root (detects if we are root already)
runAsRoot() {
  if [ $EUID -ne 0 ] && [ $USE_SUDO = "true" ]; then
    sudo "$@"
  else
    "$@"
  fi
}

# verifySupported checks that the os/arch combination is supported for
# binary builds.
verifySupported() {
  local supported="darwin-amd64\ndarwin-arm64\nlinux-amd64\nlinux-arm\nlinux-arm64\nwindows-amd64"
  if ! echo "${supported}" | grep -q "${OS}-${ARCH}"; then
    echo "No prebuilt binary for ${OS}-${ARCH}."
    echo "To build from source, go to $REPO_URL"
    exit 1
  fi

  if ! type "curl" > /dev/null && ! type "wget" > /dev/null; then
    echo "Either curl or wget is required"
    exit 1
  fi

  if [ "${VERIFY_CHECKSUM}" == "true" ] && [ "${HAS_OPENSSL}" != "true" ]; then
    echo "In order to verify checksum, openssl must first be installed."
    echo "Please install openssl or set VERIFY_CHECKSUM=false in your environment."
    exit 1
  fi
}

# checkFuseMLInstallerInstalledVersion checks which version of fusemnl-installer
# is installed and if it needs to be changed.
checkFuseMLInstallerInstalledVersion() {
  if [[ -f "${FUSEML_INSTALLER_INSTALL_DIR}/${APP_NAME}" ]]; then
    local version
    version=$(fuseml-installer -v | cut -d" " -f3)
    if [[ "$version" == "$TAG" ]]; then
      echo "fuseml-installer ${version} is already ${DESIRED_VERSION:-latest}"
      return 0
    else
      echo "fuseml-installer ${TAG} is available. Changing from version ${version}."
      return 1
    fi
  else
    return 1
  fi
}

# checkTagProvided checks whether TAG has provided as an environment variable so we can skip checkLatestVersion.
checkTagProvided() {
  if [ -n "$TAG" ]; then
    DESIRED_VERSION=$TAG
    return 0
  fi
  return 1
}

# checkLatestVersion grabs the latest version string from the releases
checkLatestVersion() {
  local latest_release_url="$REPO_URL/releases/latest"
  if type "curl" > /dev/null; then
    TAG=$(curl -Ls -o /dev/null -w '%{url_effective}' $latest_release_url | grep -oE "[^/]+$" )
  elif type "wget" > /dev/null; then
    TAG=$(wget $latest_release_url --server-response -O /dev/null 2>&1 | awk '/^\s*Location: /{DEST=$2} END{ print DEST}' | grep -oE "[^/]+$")
  fi
}

# downloadFile downloads the latest binary package and also the checksum
# for that binary.
downloadFile() {
  FUSEML_INSTALLER_DIST="fuseml-installer-$OS-$ARCH.tar.gz"
  DOWNLOAD_URL="$REPO_URL/releases/download/$TAG/$FUSEML_INSTALLER_DIST"
  FUSEML_INSTALLER_DIST_TMP_ROOT="$(mktemp -dt fuseml-installer-XXXXXX)"
  FUSEML_INSTALLER_TMP_FILE="$FUSEML_INSTALLER_DIST_TMP_ROOT/$FUSEML_INSTALLER_DIST"
  if type "curl" > /dev/null; then
    curl -SsL "$DOWNLOAD_URL" -o "$FUSEML_INSTALLER_TMP_FILE"
    curl -SsL "$DOWNLOAD_URL.sha256" -o "$FUSEML_INSTALLER_TMP_FILE.sha256"
  elif type "wget" > /dev/null; then
    wget -q -O "$FUSEML_INSTALLER_TMP_FILE" "$DOWNLOAD_URL"
    wget -q -O "$FUSEML_INSTALLER_TMP_FILE.sha256" "$DOWNLOAD_URL.sha256"
  fi
}

# verifyFile verifies the SHA256 checksum of the binary package
# (depending on settings in environment).
verifyFile() {
  if [ "${VERIFY_CHECKSUM}" == "true" ]; then
    verifyChecksum
  fi
}

# installFile unpacks and installs the binary.
installFile() {
  echo "Preparing to install $APP_NAME into ${FUSEML_INSTALLER_INSTALL_DIR}"
  tar xzf "$FUSEML_INSTALLER_TMP_FILE" -C "$FUSEML_INSTALLER_DIST_TMP_ROOT"
  if ! (runAsRoot rm -f "$FUSEML_INSTALLER_INSTALL_DIR/$APP_NAME" ; \
     runAsRoot cp "$FUSEML_INSTALLER_DIST_TMP_ROOT/$APP_NAME" "$FUSEML_INSTALLER_INSTALL_DIR/$APP_NAME${EXTENSION}"); then
    return 1
  fi
  echo "$APP_NAME installed into $FUSEML_INSTALLER_INSTALL_DIR/$APP_NAME"
}

# verifyChecksum verifies the SHA256 checksum of the binary package.
verifyChecksum() {
  printf "Verifying checksum... "
  local sum
  local expected_sum
  sum=$(openssl sha1 -sha256 "${FUSEML_INSTALLER_TMP_FILE}" | awk '{print $2}')
  expected_sum=$(grep -i "${FUSEML_INSTALLER_DIST}" "${FUSEML_INSTALLER_TMP_FILE}.sha256" | cut -f 1 -d " ")
  if [ "$sum" != "$expected_sum" ]; then
    echo "SHA sum of ${HYPPER_TMP_FILE} does not match. Aborting."
    exit 1
  fi
  echo "Done."
}

# fail_trap is executed if an error occurs.
fail_trap() {
  result=$?
  if [ "$result" != "0" ]; then
    if [[ -n "$INPUT_ARGUMENTS" ]]; then
      echo "Failed to install $APP_NAME with the arguments provided: $INPUT_ARGUMENTS"
      help
    else
      echo "Failed to install $APP_NAME"
    fi
    echo -e "\tFor support, go to $REPO_URL"
  fi
  cleanup
  exit $result
}

# testVersion tests the installed client to make sure it is working.
testVersion() {
  if ! command -v $APP_NAME &> /dev/null; then
    echo "$APP_NAME not found. Is $FUSEML_INSTALLER_INSTALL_DIR on your "'$PATH?'
    exit 1
  fi
  echo "Run '$APP_NAME --help' to see what you can do with it."
}

# help provides possible cli installation arguments
help () {
  echo "Accepted cli arguments are:"
  echo -e "\t[--help|-h ] ->> prints this help"
  echo -e "\t[--no-sudo]  ->> install without sudo"
}

# cleanup temporary files
cleanup() {
  if [[ -d "${FUSEML_INSTALLER_DIST_TMP_ROOT:-}" ]]; then
    rm -rf "$FUSEML_INSTALLER_DIST_TMP_ROOT"
  fi
}

# Execution

#Stop execution on any error
trap "fail_trap" EXIT
set -e

# Parsing input arguments (if any)
export INPUT_ARGUMENTS="$*"
set -u
while [[ $# -gt 0 ]]; do
  case $1 in
    '--no-sudo')
       USE_SUDO="false"
       ;;
    '--help'|-h)
       help
       exit 0
       ;;
    *) exit 1
       ;;
  esac
  shift
done
set +u

initArch
initOS
verifySupported
checkTagProvided || checkLatestVersion
if ! checkFuseMLInstallerInstalledVersion; then
  downloadFile
  verifyFile
  installFile
fi
testVersion
cleanup
