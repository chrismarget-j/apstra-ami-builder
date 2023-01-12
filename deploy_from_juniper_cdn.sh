#!/bin/bash -eu
set -o pipefail

REPOROOT=$(cd "$(dirname $0)"; pwd)
TFROOT=${REPOROOT}/terraform

INSTANCE_TYPE="t3a.small"

DOWNLOAD_PAGE="https://support.juniper.net/support/downloads/?p=afc"
ROLE_NAME_TFSTATE_PATH=".values.outputs.apstra_ami_builder_role_name.value"

URI_REGEX='^(([^:/?#]+):)?(//((([^:/?#]+)@)?([^:/?#]+)(:([0-9]+))?))?(/([^?#]*))(\?([^#]*))?(#(.*))?'

die() {
  echo "$1"
  exit $2
}

check_aws_creds() {
  echo -n "Checking for valid aws credentials..."
  ACCOUNT_ID=$(aws sts get-caller-identity | jq -r ".Account")
  echo "  Done."
}

find_image() {
  echo -n "Finding AMI..."
  IMAGE_ID=$(aws ec2 describe-images --owners=self --filters "Name=name,Values=apstra-ami-builder *" | jq -r '.Images | sort_by(.CreationDate) | last .ImageId')
  if [ -z "$IMAGE_ID" ] || [ "$IMAGE_ID" == "null" ]
  then
    die "AMI not found"
  fi
  echo "  Done."
}

prompt_for_link() {
  echo ""
  echo "Please visit $DOWNLOAD_PAGE and click"
  echo "the link for \"Apstra VM Image for VMware ESXi\" (an \"ova\" file). Then copy"
  echo "tokenized download link and paste it here."
  echo ""
  if DEFAULT_LINK_PROMPT=$(printenv APSTRA_DOWNLOAD_LINK); then
    echo -n "Download link [$DEFAULT_LINK_PROMPT]: "
  else
    echo -n "Download link: "
  fi
}

fetch_tf_state() {
  echo -n "Fetching terraform state..."
  if ! STATE=$(terraform -chdir=${TFROOT} show -json); then
    die "  Error fetching terraform state - is the project deployed?" $?
  fi
  echo "  Done."
}

parse_tf_state() {
  echo -n "Parsing terraform state..."
  ROLE=$(jq -r "$ROLE_NAME_TFSTATE_PATH" <<< $STATE)
  if [ "$ROLE" == "null" ]; then
    die "  EC2 role name not found in terrraform state - is the project deployed?" $?
  fi
  echo "  Done."
}

prompt_for_link() {
  echo ""
  echo "Please visit $DOWNLOAD_PAGE and click"
  echo "the link for \"Apstra VM Image for VMware ESXi\" (an \"ova\" file). Then copy"
  echo "tokenized download link and paste it here."
  echo ""
  if DEFAULT_LINK_PROMPT=$(printenv APSTRA_DOWNLOAD_LINK); then
    echo -n "Download link [$DEFAULT_LINK_PROMPT]: "
  else
    echo -n "Download link: "
  fi
}

read_uri() {
  read -r URI
  if [ "$URI" == "" ]; then
    URI=$DEFAULT_LINK_PROMPT
  fi

  [[ "$URI" =~ $URI_REGEX ]] || die "error parsing link" $?
  OVA_PATH="${BASH_REMATCH[10]}"
  OVA=$(basename $OVA_PATH)

  remove_query_string "$URI"
}

remove_query_string() {
  [[ "$1" =~ $URI_REGEX ]] || die "error parsing link" $?

  # extract the query string length so we can trim it off
  if [ ${#BASH_REMATCH[@]} -ge 13 ]; then
    TRIM="${#BASH_REMATCH[12]}"
  else
    TRIM=0
  fi

  SHORT_URI="${URI:0:((${#URI}-$TRIM))}"
}

make_user_data() {
  local breadcrumbs
  local yaml

  breadcrumbs="{}"
  breadcrumbs=$(jq -c ".|.[\"ova_url\"]=\"$URI\"" <<< $breadcrumbs)

  yaml=()
  yaml+=("#cloud-config\n")
  yaml+=("write_files:\n")
  yaml+=("- encoding: b64\n")
  yaml+=("  content: $(base64 <<< "$breadcrumbs")\n")
  yaml+=("  owner: root:root\n")
  yaml+=("  path: /var/local/ci-breadcrumbs.json\n")
  yaml+=("  permissions: '0644'\n")

  USER_DATA=$(IFS=; echo -e "${yaml[*]}")
}

check_aws_creds
find_image
fetch_tf_state
parse_tf_state
prompt_for_link
read_uri
make_user_data

INSTANCE_INFO=$(aws ec2 run-instances \
  --image-id "$IMAGE_ID" \
  --instance-type "$INSTANCE_TYPE" \
  --iam-instance-profile "{\"Name\": \"$ROLE\"}" \
  --instance-initiated-shutdown-behavior terminate \
  --user-data file:///dev/stdin \
  <<< "$USER_DATA")

INSTANCE_ID=$(jq -r '.Instances[0].InstanceId' <<< "$INSTANCE_INFO")

echo "Instance $INSTANCE_ID is building the AMI..."
