#!/usr/bin/env bash

# Apache License Version 2.0, January 2004
# https://github.com/codecov/codecov-bash/blob/master/LICENSE

set -e +o pipefail

VERSION="1.0.3"

codecov_flags=( )
url="https://codecov.io"
env="$CODECOV_ENV"
service=""
token=""
search_in=""
# shellcheck disable=SC2153
flags="$CODECOV_FLAGS"
exit_with=0
curlargs=""
curlawsargs=""
dump="0"
clean="0"
curl_s="-s"
name="$CODECOV_NAME"
include_cov=""
exclude_cov=""
ddp="$HOME/Library/Developer/Xcode/DerivedData"
xp=""
files=""
save_to=""
direct_file_upload=""
cacert="$CODECOV_CA_BUNDLE"
gcov_ignore="-not -path './bower_components/**' -not -path './node_modules/**' -not -path './vendor/**'"
gcov_include=""

ft_gcov="1"
ft_coveragepy="1"
ft_fix="1"
ft_search="1"
ft_s3="1"
ft_network="1"
ft_xcodellvm="1"
ft_xcodeplist="0"
ft_gcovout="1"
ft_html="0"
ft_yaml="0"

_git_root=$(git rev-parse --show-toplevel 2>/dev/null || hg root 2>/dev/null || echo "$PWD")
git_root="$_git_root"
remote_addr=""
if [ "$git_root" = "$PWD" ];
then
  git_root="."
fi

branch_o=""
build_o=""
commit_o=""
pr_o=""
prefix_o=""
network_filter_o=""
search_in_o=""
slug_o=""
tag_o=""
url_o=""
git_ls_files_recurse_submodules_o=""
package="bash"

commit="$VCS_COMMIT_ID"
branch="$VCS_BRANCH_NAME"
pr="$VCS_PULL_REQUEST"
slug="$VCS_SLUG"
tag="$VCS_TAG"
build_url="$CI_BUILD_URL"
build="$CI_BUILD_ID"
job="$CI_JOB_ID"

beta_xcode_partials=""

proj_root="$git_root"
gcov_exe="gcov"
gcov_arg=""

b="\033[0;36m"
g="\033[0;32m"
r="\033[0;31m"
e="\033[0;90m"
y="\033[0;33m"
x="\033[0m"

show_help() {
cat << EOF

                Codecov Bash $VERSION

          Global report uploading tool for Codecov
       Documentation at https://docs.codecov.io/docs
    Contribute at https://github.com/codecov/codecov-bash


    -h          Display this help and exit
    -f FILE     Target file(s) to upload

                 -f "path/to/file"     only upload this file
                                       skips searching unless provided patterns below

                 -f '!*.bar'           ignore all files at pattern *.bar
                 -f '*.foo'            include all files at pattern *.foo
                 Must use single quotes.
                 This is non-exclusive, use -s "*.foo" to match specific paths.

    -s DIR       Directory to search for coverage reports.
                 Already searches project root and artifact folders.
    -t TOKEN     Set the private repository token
                 (option) set environment variable CODECOV_TOKEN=:uuid

                 -t @/path/to/token_file
                 -t uuid

    -n NAME      Custom defined name of the upload. Visible in Codecov UI

    -e ENV       Specify environment variables to be included with this build
                 Also accepting environment variables: CODECOV_ENV=VAR,VAR2

                 -e VAR,VAR2

    -k prefix    Prefix filepaths to help resolve path fixing

    -i prefix    Only include files in the network with a certain prefix. Useful for upload-specific path fixing

    -X feature   Toggle functionalities

                 -X gcov          Disable gcov
                 -X coveragepy    Disable python coverage
                 -X fix           Disable report fixing
                 -X search        Disable searching for reports
                 -X xcode         Disable xcode processing
                 -X network       Disable uploading the file network
                 -X gcovout       Disable gcov output
                 -X html          Enable coverage for HTML files
                 -X recursesubs   Enable recurse submodules in git projects when searching for source files
                 -X yaml          Enable coverage for YAML files

    -N           The commit SHA of the parent for which you are uploading coverage. If not present,
                 the parent will be determined using the API of your repository provider.
                 When using the repository provider's API, the parent is determined via finding
                 the closest ancestor to the commit.

    -R root dir  Used when not in git/hg project to identify project root directory
    -F flag      Flag the upload to group coverage metrics

                 -F unittests        This upload is only unittests
                 -F integration      This upload is only integration tests
                 -F ui,chrome        This upload is Chrome - UI tests

    -c           Move discovered coverage reports to the trash
    -z FILE      Upload specified file directly to Codecov and bypass all report generation.
                 This is intended to be used only with a pre-formatted Codecov report and is
                 not expected to work under any other circumstances.
    -Z           Exit with 1 if not successful. Default will Exit with 0

    -- xcode --
    -D           Custom Derived Data Path for Coverage.profdata and gcov processing
                 Default '~/Library/Developer/Xcode/DerivedData'
    -J           Specify packages to build coverage. Uploader will only build these packages.
                 This can significantly reduces time to build coverage reports.

                 -J 'MyAppName'      Will match "MyAppName" and "MyAppNameTests"
                 -J '^ExampleApp$'   Will match only "ExampleApp" not "ExampleAppTests"

    -- gcov --
    -g GLOB      Paths to ignore during gcov gathering
    -G GLOB      Paths to include during gcov gathering
    -p dir       Project root directory
                 Also used when preparing gcov
    -x gcovexe   gcov executable to run. Defaults to 'gcov'
    -a gcovargs  extra arguments to pass to gcov

    -- Override CI Environment Variables --
       These variables are automatically detected by popular CI providers

    -B branch    Specify the branch name
    -C sha       Specify the commit sha
    -P pr        Specify the pull request number
    -b build     Specify the build number
    -T tag       Specify the git tag

    -- Enterprise --
    -u URL       Set the target url for Enterprise customers
                 Not required when retrieving the bash uploader from your CCE
                 (option) Set environment variable CODECOV_URL=https://my-hosted-codecov.com
    -r SLUG      owner/repo slug used instead of the private repo token in Enterprise
                 (option) set environment variable CODECOV_SLUG=:owner/:repo
                 (option) set in your codecov.yml "codecov.slug"
    -S PATH      File path to your cacert.pem file used to verify ssl with Codecov Enterprise (optional)
                 (option) Set environment variable: CODECOV_CA_BUNDLE="/path/to/ca.pem"
    -U curlargs  Extra curl arguments to communicate with Codecov. e.g., -U "--proxy http://http-proxy"
    -A curlargs  Extra curl arguments to communicate with AWS.

    -- Debugging --
    -d           Don't upload, but dump upload file to stdout
    -q PATH      Write upload file to path
    -K           Remove color from the output
    -v           Verbose mode

EOF
}


say() {
  echo -e "$1"
}


urlencode() {
  echo "$1" | curl -Gso /dev/null -w "%{url_effective}" --data-urlencode @- "" | cut -c 3- | sed -e 's/%0A//'
}

swiftcov() {
  _dir=$(dirname "$1" | sed 's/\(Build\).*/\1/g')
  for _type in app framework xctest
  do
    find "$_dir" -name "*.$_type" | while read -r f
    do
      _proj=${f##*/}
      _proj=${_proj%."$_type"}
      if [ "$2" = "" ] || [ "$(echo "$_proj" | grep -i "$2")" != "" ];
      then
        say "    $g+$x Building reports for $_proj $_type"
        dest=$([ -f "$f/$_proj" ] && echo "$f/$_proj" || echo "$f/Contents/MacOS/$_proj")
        # shellcheck disable=SC2001
        _proj_name=$(echo "$_proj" | sed -e 's/[[:space:]]//g')
        # shellcheck disable=SC2086
        xcrun llvm-cov show $beta_xcode_partials -instr-profile "$1" "$dest" > "$_proj_name.$_type.coverage.txt" \
         || say "    ${r}x>${x} llvm-cov failed to produce results for $dest"
      fi
    done
  done
}


# Credits to: https://gist.github.com/pkuczynski/8665367
parse_yaml() {
   local prefix=$2
   local s='[[:space:]]*' w='[a-zA-Z0-9_]*'
   local fs
   fs=$(echo @|tr @ '\034')
   sed -ne "s|^\($s\)\($w\)$s:$s\"\(.*\)\"$s\$|\1$fs\2$fs\3|p" \
        -e "s|^\($s\)\($w\)$s:$s\(.*\)$s\$|\1$fs\2$fs\3|p" "$1" |
   awk -F"$fs" '{
      indent = length($1)/2;
      vname[indent] = $2;
      for (i in vname) {if (i > indent) {delete vname[i]}}
      if (length($3) > 0) {
         vn=""; if (indent > 0) {vn=(vn)(vname[0])("_")}
         printf("%s%s%s=\"%s\"\n", "'"$prefix"'",vn, $2, $3);
      }
   }'
}

if [ $# != 0 ];
then
  while getopts "a:A:b:B:cC:dD:e:f:F:g:G:hi:J:k:Kn:p:P:Q:q:r:R:s:S:t:T:u:U:vx:X:Zz:N:-" o
  do
    codecov_flags+=( "$o" )
    case "$o" in
      "-")
        echo -e "${r}Long options are not supported${x}"
        exit 2
        ;;
      "?")
        ;;
      "N")
        parent=$OPTARG
        ;;
      "a")
        gcov_arg=$OPTARG
        ;;
      "A")
        curlawsargs="$OPTARG"
        ;;
      "b")
        build_o="$OPTARG"
        ;;
      "B")
        branch_o="$OPTARG"
        ;;
      "c")
        clean="1"
        ;;
      "C")
        commit_o="$OPTARG"
        ;;
      "d")
        dump="1"
        ;;
      "D")
        ddp="$OPTARG"
        ;;
      "e")
        env="$env,$OPTARG"
        ;;
      "f")
        if [ "${OPTARG::1}" = "!" ];
        then
          exclude_cov="$exclude_cov -not -path '${OPTARG:1}'"

        elif [[ "$OPTARG" = *"*"* ]];
        then
          include_cov="$include_cov -or -path '$OPTARG'"

        else
          ft_search=0
          if [ "$files" = "" ];
          then
            files="$OPTARG"
          else
            files="$files
$OPTARG"
          fi
        fi
        ;;
      "F")
        if [ "$flags" = "" ];
        then
          flags="$OPTARG"
        else
          flags="$flags,$OPTARG"
        fi
        ;;
      "g")
        gcov_ignore="$gcov_ignore -not -path '$OPTARG'"
        ;;
      "G")
        gcov_include="$gcov_include -path '$OPTARG'"
        ;;
      "h")
        show_help
        exit 0;
        ;;
      "i")
        network_filter_o="$OPTARG"
        ;;
      "J")
        ft_xcodellvm="1"
        ft_xcodeplist="0"
        if [ "$xp" = "" ];
        then
          xp="$OPTARG"
        else
          xp="$xp\|$OPTARG"
        fi
        ;;
      "k")
        prefix_o=$(echo "$OPTARG" | sed -e 's:^/*::' -e 's:/*$::')
        ;;
      "K")
        b=""
        g=""
        r=""
        e=""
        x=""
        ;;
      "n")
        name="$OPTARG"
        ;;
      "p")
        proj_root="$OPTARG"
        ;;
      "P")
        pr_o="$OPTARG"
        ;;
      "Q")
        # this is only meant for Codecov packages to overwrite
        package="$OPTARG"
        ;;
      "q")
        save_to="$OPTARG"
        ;;
      "r")
        slug_o="$OPTARG"
        ;;
      "R")
        git_root="$OPTARG"
        ;;
      "s")
        if [ "$search_in_o" = "" ];
        then
          search_in_o="$OPTARG"
        else
          search_in_o="$search_in_o $OPTARG"
        fi
        ;;
      "S")
        # shellcheck disable=SC2089
        cacert="--cacert \"$OPTARG\""
        ;;
      "t")
        if [ "${OPTARG::1}" = "@" ];
        then
          token=$(< "${OPTARG:1}" tr -d ' \n')
        else
          token="$OPTARG"
        fi
        ;;
      "T")
        tag_o="$OPTARG"
        ;;
      "u")
        url_o=$(echo "$OPTARG" | sed -e 's/\/$//')
        ;;
      "U")
        curlargs="$OPTARG"
        ;;
      "v")
        set -x
        curl_s=""
        ;;
      "x")
        gcov_exe=$OPTARG
        ;;
      "X")
        if [ "$OPTARG" = "gcov" ];
        then
          ft_gcov="0"
        elif [ "$OPTARG" = "coveragepy" ] || [ "$OPTARG" = "py" ];
        then
          ft_coveragepy="0"
        elif [ "$OPTARG" = "gcovout" ];
        then
          ft_gcovout="0"
        elif [ "$OPTARG" = "xcodellvm" ];
        then
          ft_xcodellvm="1"
          ft_xcodeplist="0"
        elif [ "$OPTARG" = "fix" ] || [ "$OPTARG" = "fixes" ];
        then
          ft_fix="0"
        elif [ "$OPTARG" = "xcode" ];
        then
          ft_xcodellvm="0"
          ft_xcodeplist="0"
        elif [ "$OPTARG" = "search" ];
        then
          ft_search="0"
        elif [ "$OPTARG" = "xcodepartials" ];
        then
          beta_xcode_partials="-use-color"
        elif [ "$OPTARG" = "network" ];
        then
          ft_network="0"
        elif [ "$OPTARG" = "s3" ];
        then
          ft_s3="0"
        elif [ "$OPTARG" = "html" ];
        then
          ft_html="1"
        elif [ "$OPTARG" = "recursesubs" ];
        then
          git_ls_files_recurse_submodules_o="--recurse-submodules"
        elif [ "$OPTARG" = "yaml" ];
        then
          ft_yaml="1"
        fi
        ;;
      "Z")
        exit_with=1
        ;;
      "z")
        direct_file_upload="$OPTARG"
        ft_gcov="0"
        ft_coveragepy="0"
        ft_fix="0"
        ft_search="0"
        ft_network="0"
        ft_xcodellvm="0"
        ft_gcovout="0"
        include_cov=""
        ;;
      *)
        echo -e "${r}Unexpected flag not supported${x}"
        ;;
    esac
  done
fi

say "
  _____          _
 / ____|        | |
| |     ___   __| | ___  ___ _____   __
| |    / _ \\ / _\` |/ _ \\/ __/ _ \\ \\ / /
| |___| (_) | (_| |  __/ (_| (_) \\ V /
 \\_____\\___/ \\__,_|\\___|\\___\\___/ \\_/
                              Bash-$VERSION

"

# check for installed tools
# git/hg
if [ "$direct_file_upload" = "" ];
then
  if [ -x "$(command -v git)" ];
  then
    say "$b==>$x $(git --version) found"
  else
    say "$y==>$x git not installed, testing for mercurial"
    if [ -x "$(command -v hg)" ];
    then
      say "$b==>$x $(hg --version) found"
    else
      say "$r==>$x git nor mercurial are installed. Uploader may fail or have unintended consequences"
    fi
  fi
fi
# curl
if [ -x "$(command -v curl)" ];
then
  say "$b==>$x $(curl --version)"
else
  say "$r==>$x curl not installed. Exiting."
  exit ${exit_with};
fi

search_in="$proj_root"

#shellcheck disable=SC2154
if [ "$JENKINS_URL" != "" ];
then
  say "$e==>$x Jenkins CI detected."
  # https://wiki.jenkins-ci.org/display/JENKINS/Building+a+software+project
  # https://wiki.jenkins-ci.org/display/JENKINS/GitHub+pull+request+builder+plugin#GitHubpullrequestbuilderplugin-EnvironmentVariables
  service="jenkins"

  # shellcheck disable=SC2154
  if [ "$ghprbSourceBranch" != "" ];
  then
     branch="$ghprbSourceBranch"
  elif [ "$GIT_BRANCH" != "" ];
  then
     branch="$GIT_BRANCH"
  elif [ "$BRANCH_NAME" != "" ];
  then
    branch="$BRANCH_NAME"
  fi

  # shellcheck disable=SC2154
  if [ "$ghprbActualCommit" != "" ];
  then
    commit="$ghprbActualCommit"
  elif [ "$GIT_COMMIT" != "" ];
  then
    commit="$GIT_COMMIT"
  fi

  # shellcheck disable=SC2154
  if [ "$ghprbPullId" != "" ];
  then
    pr="$ghprbPullId"
  elif [ "$CHANGE_ID" != "" ];
  then
    pr="$CHANGE_ID"
  fi

  build="$BUILD_NUMBER"
  # shellcheck disable=SC2153
  build_url=$(urlencode "$BUILD_URL")

elif [ "$CI" = "true" ] && [ "$TRAVIS" = "true" ] && [ "$SHIPPABLE" != "true" ];
then
  say "$e==>$x Travis CI detected."
  # https://docs.travis-ci.com/user/environment-variables/
  service="travis"
  commit="${TRAVIS_PULL_REQUEST_SHA:-$TRAVIS_COMMIT}"
  build="$TRAVIS_JOB_NUMBER"
  pr="$TRAVIS_PULL_REQUEST"
  job="$TRAVIS_JOB_ID"
  slug="$TRAVIS_REPO_SLUG"
  env="$env,TRAVIS_OS_NAME"
  tag="$TRAVIS_TAG"
  if [ "$TRAVIS_BRANCH" != "$TRAVIS_TAG" ];
  then
    branch="${TRAVIS_PULL_REQUEST_BRANCH:-$TRAVIS_BRANCH}"
  fi

  language=$(compgen -A variable | grep "^TRAVIS_.*_VERSION$" | head -1)
  if [ "$language" != "" ];
  then
    env="$env,${!language}"
  fi

elif [ "$CODEBUILD_CI" = "true" ];
then
  say "$e==>$x AWS Codebuild detected."
  # https://docs.aws.amazon.com/codebuild/latest/userguide/build-env-ref-env-vars.html
  service="codebuild"
  commit="$CODEBUILD_RESOLVED_SOURCE_VERSION"
  build="$CODEBUILD_BUILD_ID"
  branch="$(echo "$CODEBUILD_WEBHOOK_HEAD_REF" | sed 's/^refs\/heads\///')"
  if [ "${CODEBUILD_SOURCE_VERSION/pr}" = "$CODEBUILD_SOURCE_VERSION" ] ; then
    pr="false"
  else
    pr="$(echo "$CODEBUILD_SOURCE_VERSION" | sed 's/^pr\///')"
  fi
  job="$CODEBUILD_BUILD_ID"
  slug="$(echo "$CODEBUILD_SOURCE_REPO_URL" | sed 's/^.*:\/\/[^\/]*\///' | sed 's/\.git$//')"

elif [ "$CI" = "true" ] && [ "$CI_NAME" = "codeship" ];
then
  say "$e==>$x Codeship CI detected."
  # https://www.codeship.io/documentation/continuous-integration/set-environment-variables/
  service="codeship"
  branch="$CI_BRANCH"
  build="$CI_BUILD_NUMBER"
  build_url=$(urlencode "$CI_BUILD_URL")
  commit="$CI_COMMIT_ID"

elif [ -n "$CF_BUILD_URL" ] && [ -n "$CF_BUILD_ID" ];
then
  say "$e==>$x Codefresh CI detected."
  # https://docs.codefresh.io/v1.0/docs/variables
  service="codefresh"
  branch="$CF_BRANCH"
  build="$CF_BUILD_ID"
  build_url=$(urlencode "$CF_BUILD_URL")
  commit="$CF_REVISION"

elif [ "$TEAMCITY_VERSION" != "" ];
then
  say "$e==>$x TeamCity CI detected."
  # https://confluence.jetbrains.com/display/TCD8/Predefined+Build+Parameters
  # https://confluence.jetbrains.com/plugins/servlet/mobile#content/view/74847298
  if [ "$TEAMCITY_BUILD_BRANCH" = '' ];
  then
    echo "    Teamcity does not automatically make build parameters available as environment variables."
    echo "    Add the following environment parameters to the build configuration"
    echo "    env.TEAMCITY_BUILD_BRANCH = %teamcity.build.branch%"
    echo "    env.TEAMCITY_BUILD_ID = %teamcity.build.id%"
    echo "    env.TEAMCITY_BUILD_URL = %teamcity.serverUrl%/viewLog.html?buildId=%teamcity.build.id%"
    echo "    env.TEAMCITY_BUILD_COMMIT = %system.build.vcs.number%"
    echo "    env.TEAMCITY_BUILD_REPOSITORY = %vcsroot.<YOUR TEAMCITY VCS NAME>.url%"
  fi
  service="teamcity"
  branch="$TEAMCITY_BUILD_BRANCH"
  build="$TEAMCITY_BUILD_ID"
  build_url=$(urlencode "$TEAMCITY_BUILD_URL")
  if [ "$TEAMCITY_BUILD_COMMIT" != "" ];
  then
    commit="$TEAMCITY_BUILD_COMMIT"
  else
    commit="$BUILD_VCS_NUMBER"
  fi
  remote_addr="$TEAMCITY_BUILD_REPOSITORY"

elif [ "$CI" = "true" ] && [ "$CIRCLECI" = "true" ];
then
  say "$e==>$x Circle CI detected."
  # https://circleci.com/docs/environment-variables
  service="circleci"
  branch="$CIRCLE_BRANCH"
  build="$CIRCLE_BUILD_NUM"
  job="$CIRCLE_NODE_INDEX"
  if [ "$CIRCLE_PROJECT_REPONAME" != "" ];
  then
    slug="$CIRCLE_PROJECT_USERNAME/$CIRCLE_PROJECT_REPONAME"
  else
    # git@github.com:owner/repo.git
    slug="${CIRCLE_REPOSITORY_URL##*:}"
    # owner/repo.git
    slug="${slug%%.git}"
  fi
  pr="${CIRCLE_PULL_REQUEST##*/}"
  commit="$CIRCLE_SHA1"
  search_in="$search_in $CIRCLE_ARTIFACTS $CIRCLE_TEST_REPORTS"

elif [ "$BUDDYBUILD_BRANCH" != "" ];
then
  say "$e==>$x buddybuild detected"
  # http://docs.buddybuild.com/v6/docs/custom-prebuild-and-postbuild-steps
  service="buddybuild"
  branch="$BUDDYBUILD_BRANCH"
  build="$BUDDYBUILD_BUILD_NUMBER"
  build_url="https://dashboard.buddybuild.com/public/apps/$BUDDYBUILD_APP_ID/build/$BUDDYBUILD_BUILD_ID"
  # BUDDYBUILD_TRIGGERED_BY
  if [ "$ddp" = "$HOME/Library/Developer/Xcode/DerivedData" ];
  then
    ddp="/private/tmp/sandbox/${BUDDYBUILD_APP_ID}/bbtest"
  fi

elif [ "${bamboo_planRepository_revision}" != "" ];
then
  say "$e==>$x Bamboo detected"
  # https://confluence.atlassian.com/bamboo/bamboo-variables-289277087.html#Bamboovariables-Build-specificvariables
  service="bamboo"
  commit="${bamboo_planRepository_revision}"
  # shellcheck disable=SC2154
  branch="${bamboo_planRepository_branch}"
  # shellcheck disable=SC2154
  build="${bamboo_buildNumber}"
  # shellcheck disable=SC2154
  build_url="${bamboo_buildResultsUrl}"
  # shellcheck disable=SC2154
  remote_addr="${bamboo_planRepository_repositoryUrl}"

elif [ "$CI" = "true" ] && [ "$BITRISE_IO" = "true" ];
then
  # http://devcenter.bitrise.io/faq/available-environment-variables/
  say "$e==>$x Bitrise CI detected."
  service="bitrise"
  branch="$BITRISE_GIT_BRANCH"
  build="$BITRISE_BUILD_NUMBER"
  build_url=$(urlencode "$BITRISE_BUILD_URL")
  pr="$BITRISE_PULL_REQUEST"
  if [ "$GIT_CLONE_COMMIT_HASH" != "" ];
  then
    commit="$GIT_CLONE_COMMIT_HASH"
  fi

elif [ "$CI" = "true" ] && [ "$SEMAPHORE" = "true" ];
then
  say "$e==>$x Semaphore CI detected."
# https://docs.semaphoreci.com/ci-cd-environment/environment-variables/#semaphore-related
  service="semaphore"
  branch="$SEMAPHORE_GIT_BRANCH"
  build="$SEMAPHORE_WORKFLOW_NUMBER"
  job="$SEMAPHORE_JOB_ID"
  pr="$PULL_REQUEST_NUMBER"
  slug="$SEMAPHORE_REPO_SLUG"
  commit="$REVISION"
  env="$env,SEMAPHORE_TRIGGER_SOURCE"

elif [ "$CI" = "true" ] && [ "$BUILDKITE" = "true" ];
then
  say "$e==>$x Buildkite CI detected."
  # https://buildkite.com/docs/guides/environment-variables
  service="buildkite"
  branch="$BUILDKITE_BRANCH"
  build="$BUILDKITE_BUILD_NUMBER"
  job="$BUILDKITE_JOB_ID"
  build_url=$(urlencode "$BUILDKITE_BUILD_URL")
  slug="$BUILDKITE_PROJECT_SLUG"
  commit="$BUILDKITE_COMMIT"
  if [[ "$BUILDKITE_PULL_REQUEST" != "false" ]]; then
    pr="$BUILDKITE_PULL_REQUEST"
  fi
  tag="$BUILDKITE_TAG"

elif [ "$CI" = "drone" ] || [ "$DRONE" = "true" ];
then
  say "$e==>$x Drone CI detected."
  # http://docs.drone.io/env.html
  # drone commits are not full shas
  service="drone.io"
  branch="$DRONE_BRANCH"
  build="$DRONE_BUILD_NUMBER"
  build_url=$(urlencode "${DRONE_BUILD_LINK}")
  pr="$DRONE_PULL_REQUEST"
  job="$DRONE_JOB_NUMBER"
  tag="$DRONE_TAG"

elif [ "$CI" = "true" ] && [ "$HEROKU_TEST_RUN_BRANCH" != "" ];
then
  say "$e==>$x Heroku CI detected."
  # https://devcenter.heroku.com/articles/heroku-ci#environment-variables
  service="heroku"
  branch="$HEROKU_TEST_RUN_BRANCH"
  build="$HEROKU_TEST_RUN_ID"
  commit="$HEROKU_TEST_RUN_COMMIT_VERSION"

elif [[ "$CI" = "true" || "$CI" = "True" ]] && [[ "$APPVEYOR" = "true" || "$APPVEYOR" = "True" ]];
then
  say "$e==>$x Appveyor CI detected."
  # http://www.appveyor.com/docs/environment-variables
  service="appveyor"
  branch="$APPVEYOR_REPO_BRANCH"
  build=$(urlencode "$APPVEYOR_JOB_ID")
  pr="$APPVEYOR_PULL_REQUEST_NUMBER"
  job="$APPVEYOR_ACCOUNT_NAME%2F$APPVEYOR_PROJECT_SLUG%2F$APPVEYOR_BUILD_VERSION"
  slug="$APPVEYOR_REPO_NAME"
  commit="$APPVEYOR_REPO_COMMIT"
  build_url=$(urlencode "${APPVEYOR_URL}/project/${APPVEYOR_REPO_NAME}/builds/$APPVEYOR_BUILD_ID/job/${APPVEYOR_JOB_ID}")

elif [ "$CI" = "true" ] && [ "$WERCKER_GIT_BRANCH" != "" ];
then
  say "$e==>$x Wercker CI detected."
  # http://devcenter.wercker.com/articles/steps/variables.html
  service="wercker"
  branch="$WERCKER_GIT_BRANCH"
  build="$WERCKER_MAIN_PIPELINE_STARTED"
  slug="$WERCKER_GIT_OWNER/$WERCKER_GIT_REPOSITORY"
  commit="$WERCKER_GIT_COMMIT"

elif [ "$CI" = "true" ] && [ "$MAGNUM" = "true" ];
then
  say "$e==>$x Magnum CI detected."
  # https://magnum-ci.com/docs/environment
  service="magnum"
  branch="$CI_BRANCH"
  build="$CI_BUILD_NUMBER"
  commit="$CI_COMMIT"

elif [ "$SHIPPABLE" = "true" ];
then
  say "$e==>$x Shippable CI detected."
  # http://docs.shippable.com/ci_configure/
  service="shippable"
  # shellcheck disable=SC2153
  branch=$([ "$HEAD_BRANCH" != "" ] && echo "$HEAD_BRANCH" || echo "$BRANCH")
  build="$BUILD_NUMBER"
  build_url=$(urlencode "$BUILD_URL")
  pr="$PULL_REQUEST"
  slug="$REPO_FULL_NAME"
  # shellcheck disable=SC2153
  commit="$COMMIT"

elif [ "$TDDIUM" = "true" ];
then
  say "Solano CI detected."
  # http://docs.solanolabs.com/Setup/tddium-set-environment-variables/
  service="solano"
  commit="$TDDIUM_CURRENT_COMMIT"
  branch="$TDDIUM_CURRENT_BRANCH"
  build="$TDDIUM_TID"
  pr="$TDDIUM_PR_ID"

elif [ "$GREENHOUSE" = "true" ];
then
  say "$e==>$x Greenhouse CI detected."
  # http://docs.greenhouseci.com/docs/environment-variables-files
  service="greenhouse"
  branch="$GREENHOUSE_BRANCH"
  build="$GREENHOUSE_BUILD_NUMBER"
  build_url=$(urlencode "$GREENHOUSE_BUILD_URL")
  pr="$GREENHOUSE_PULL_REQUEST"
  commit="$GREENHOUSE_COMMIT"
  search_in="$search_in $GREENHOUSE_EXPORT_DIR"

elif [ "$GITLAB_CI" != "" ];
then
  say "$e==>$x GitLab CI detected."
  # http://doc.gitlab.com/ce/ci/variables/README.html
  service="gitlab"
  branch="${CI_BUILD_REF_NAME:-$CI_COMMIT_REF_NAME}"
  build="${CI_BUILD_ID:-$CI_JOB_ID}"
  remote_addr="${CI_BUILD_REPO:-$CI_REPOSITORY_URL}"
  commit="${CI_BUILD_REF:-$CI_COMMIT_SHA}"
  slug="${CI_PROJECT_PATH}"

elif [ "$GITHUB_ACTIONS" != "" ];
then
  say "$e==>$x GitHub Actions detected."
  say "    Env vars used:"
  say "      -> GITHUB_ACTIONS:    ${GITHUB_ACTIONS}"
  say "      -> GITHUB_HEAD_REF:   ${GITHUB_HEAD_REF}"
  say "      -> GITHUB_REF:        ${GITHUB_REF}"
  say "      -> GITHUB_REPOSITORY: ${GITHUB_REPOSITORY}"
  say "      -> GITHUB_RUN_ID:     ${GITHUB_RUN_ID}"
  say "      -> GITHUB_SHA:        ${GITHUB_SHA}"
  say "      -> GITHUB_WORKFLOW:   ${GITHUB_WORKFLOW}"

  # https://github.com/features/actions
  service="github-actions"

  # https://help.github.com/en/articles/virtual-environments-for-github-actions#environment-variables
  branch="${GITHUB_REF#refs/heads/}"
  if [  "$GITHUB_HEAD_REF" != "" ];
  then
    # PR refs are in the format: refs/pull/7/merge
    pr="${GITHUB_REF#refs/pull/}"
    pr="${pr%/merge}"
    branch="${GITHUB_HEAD_REF}"
  fi
  commit="${GITHUB_SHA}"
  slug="${GITHUB_REPOSITORY}"
  build="${GITHUB_RUN_ID}"
  build_url=$(urlencode "http://github.com/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}")
  job="$(urlencode "${GITHUB_WORKFLOW}")"

  # actions/checkout runs in detached HEAD
  mc=
  if [ -n "$pr" ] && [ "$pr" != false ] && [ "$commit_o" == "" ];
  then
    mc=$(git show --no-patch --format="%P" 2>/dev/null || echo "")

    if [[ "$mc" =~ ^[a-z0-9]{40}[[:space:]][a-z0-9]{40}$ ]];
    then
      mc=$(echo "$mc" | cut -d' ' -f2)
      say "    Fixing merge commit SHA $commit -> $mc"
      commit=$mc
    elif [[ "$mc" = "" ]];
    then
      say "$r->  Issue detecting commit SHA. Please run actions/checkout with fetch-depth > 1 or set to 0$x"
    fi
  fi

elif [ "$SYSTEM_TEAMFOUNDATIONSERVERURI" != "" ];
then
  say "$e==>$x Azure Pipelines detected."
  # https://docs.microsoft.com/en-us/azure/devops/pipelines/build/variables?view=vsts
  # https://docs.microsoft.com/en-us/azure/devops/pipelines/build/variables?view=azure-devops&viewFallbackFrom=vsts&tabs=yaml
  service="azure_pipelines"
  commit="$BUILD_SOURCEVERSION"
  build="$BUILD_BUILDNUMBER"
  if [  -z "$SYSTEM_PULLREQUEST_PULLREQUESTNUMBER" ];
  then
    pr="$SYSTEM_PULLREQUEST_PULLREQUESTID"
  else
    pr="$SYSTEM_PULLREQUEST_PULLREQUESTNUMBER"
  fi
  project="${SYSTEM_TEAMPROJECT}"
  server_uri="${SYSTEM_TEAMFOUNDATIONSERVERURI}"
  job="${BUILD_BUILDID}"
  branch="${BUILD_SOURCEBRANCH#"refs/heads/"}"
  build_url=$(urlencode "${SYSTEM_TEAMFOUNDATIONSERVERURI}${SYSTEM_TEAMPROJECT}/_build/results?buildId=${BUILD_BUILDID}")

  # azure/pipelines runs in detached HEAD
  mc=
  if [ -n "$pr" ] && [ "$pr" != false ];
  then
    mc=$(git show --no-patch --format="%P" 2>/dev/null || echo "")

    if [[ "$mc" =~ ^[a-z0-9]{40}[[:space:]][a-z0-9]{40}$ ]];
    then
      mc=$(echo "$mc" | cut -d' ' -f2)
      say "    Fixing merge commit SHA $commit -> $mc"
      commit=$mc
    fi
  fi

elif [ "$CI" = "true" ] && [ "$BITBUCKET_BUILD_NUMBER" != "" ];
then
  say "$e==>$x Bitbucket detected."
  # https://confluence.atlassian.com/bitbucket/variables-in-pipelines-794502608.html
  service="bitbucket"
  branch="$BITBUCKET_BRANCH"
  build="$BITBUCKET_BUILD_NUMBER"
  slug="$BITBUCKET_REPO_OWNER/$BITBUCKET_REPO_SLUG"
  job="$BITBUCKET_BUILD_NUMBER"
  pr="$BITBUCKET_PR_ID"
  commit="$BITBUCKET_COMMIT"
  # See https://jira.atlassian.com/browse/BCLOUD-19393
  if [ "${#commit}" = 12 ];
  then
    commit=$(git rev-parse "$BITBUCKET_COMMIT")
  fi

elif [ "$CI" = "true" ] && [ "$BUDDY" = "true" ];
then
  say "$e==>$x Buddy CI detected."
  # https://buddy.works/docs/pipelines/environment-variables
  service="buddy"
  branch="$BUDDY_EXECUTION_BRANCH"
  build="$BUDDY_EXECUTION_ID"
  build_url=$(urlencode "$BUDDY_EXECUTION_URL")
  commit="$BUDDY_EXECUTION_REVISION"
  pr="$BUDDY_EXECUTION_PULL_REQUEST_NO"
  tag="$BUDDY_EXECUTION_TAG"
  slug="$BUDDY_REPO_SLUG"

elif [ "$CIRRUS_CI" != "" ];
then
  say "$e==>$x Cirrus CI detected."
  # https://cirrus-ci.org/guide/writing-tasks/#environment-variables
  service="cirrus-ci"
  slug="$CIRRUS_REPO_FULL_NAME"
  branch="$CIRRUS_BRANCH"
  pr="$CIRRUS_PR"
  commit="$CIRRUS_CHANGE_IN_REPO"
  build="$CIRRUS_BUILD_ID"
  build_url=$(urlencode "https://cirrus-ci.com/task/$CIRRUS_TASK_ID")
  job="$CIRRUS_TASK_NAME"

elif [ "$DOCKER_REPO" != "" ];
then
  say "$e==>$x Docker detected."
  # https://docs.docker.com/docker-cloud/builds/advanced/
  service="docker"
  branch="$SOURCE_BRANCH"
  commit="$SOURCE_COMMIT"
  slug="$DOCKER_REPO"
  tag="$CACHE_TAG"
  env="$env,IMAGE_NAME"

else
  say "${r}x>${x} No CI provider detected."
  say "    Testing inside Docker? ${b}http://docs.codecov.io/docs/testing-with-docker${x}"
  say "    Testing with Tox? ${b}https://docs.codecov.io/docs/python#section-testing-with-tox${x}"

fi

say "    ${e}project root:${x} $git_root"

# find branch, commit, repo from git command
if [ "$GIT_BRANCH" != "" ];
then
  branch="$GIT_BRANCH"

elif [ "$branch" = "" ];
then
  branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || hg branch 2>/dev/null || echo "")
  if [ "$branch" = "HEAD" ];
  then
    branch=""
  fi
fi

if [ "$commit_o" = "" ];
then
  if [ "$GIT_COMMIT" != "" ];
  then
    commit="$GIT_COMMIT"
  elif [ "$commit" = "" ];
  then
    commit=$(git log -1 --format="%H" 2>/dev/null || hg id -i --debug 2>/dev/null | tr -d '+' || echo "")
  fi
else
  commit="$commit_o"
fi

if [ "$CODECOV_TOKEN" != "" ] && [ "$token" = "" ];
then
  say "${e}-->${x} token set from env"
  token="$CODECOV_TOKEN"
fi

if [ "$CODECOV_URL" != "" ] && [ "$url_o" = "" ];
then
  say "${e}-->${x} url set from env"
  url_o=$(echo "$CODECOV_URL" | sed -e 's/\/$//')
fi

if [ "$CODECOV_SLUG" != "" ];
then
  say "${e}-->${x} slug set from env"
  slug_o="$CODECOV_SLUG"

elif [ "$slug" = "" ];
then
  if [ "$remote_addr" = "" ];
  then
    remote_addr=$(git config --get remote.origin.url || hg paths default || echo '')
  fi
  if [ "$remote_addr" != "" ];
  then
    if echo "$remote_addr" | grep -q "//"; then
      # https
      slug=$(echo "$remote_addr" | cut -d / -f 4,5 | sed -e 's/\.git$//')
    else
      # ssh
      slug=$(echo "$remote_addr" | cut -d : -f 2 | sed -e 's/\.git$//')
    fi
  fi
  if [ "$slug" = "/" ];
  then
    slug=""
  fi
fi

yaml=$(cd "$git_root" && \
          git ls-files "*codecov.yml" "*codecov.yaml" 2>/dev/null \
       || hg locate "*codecov.yml" "*codecov.yaml" 2>/dev/null \
       || cd "$proj_root" && find . -maxdepth 1 -type f -name '*codecov.y*ml' 2>/dev/null \
       || echo '')
yaml=$(echo "$yaml" | head -1)

if [ "$yaml" != "" ];
then
  say "    ${e}Yaml found at:${x} $yaml"
  if [[ "$yaml" != /* ]]; then
    # relative path for yaml file given, assume relative to the repo root
    yaml="$git_root/$yaml"
  fi
  config=$(parse_yaml "$yaml" || echo '')

  # TODO validate the yaml here

  if [ "$(echo "$config" | grep 'codecov_token="')" != "" ] && [ "$token" = "" ];
  then
    say "${e}-->${x} token set from yaml"
    token="$(echo "$config" | grep 'codecov_token="' | sed -e 's/codecov_token="//' | sed -e 's/"\.*//')"
  fi

  if [ "$(echo "$config" | grep 'codecov_url="')" != "" ] && [ "$url_o" = "" ];
  then
    say "${e}-->${x} url set from yaml"
    url_o="$(echo "$config" | grep 'codecov_url="' | sed -e 's/codecov_url="//' | sed -e 's/"\.*//')"
  fi

  if [ "$(echo "$config" | grep 'codecov_slug="')" != "" ] && [ "$slug_o" = "" ];
  then
    say "${e}-->${x} slug set from yaml"
    slug_o="$(echo "$config" | grep 'codecov_slug="' | sed -e 's/codecov_slug="//' | sed -e 's/"\.*//')"
  fi
else
  say "    ${g}Yaml not found, that's ok! Learn more at${x} ${b}http://docs.codecov.io/docs/codecov-yaml${x}"
fi

if [ "$branch_o" != "" ];
then
  branch=$(urlencode "$branch_o")
else
  branch=$(urlencode "$branch")
fi

if [ "$slug_o" = "" ];
then
  urlencoded_slug=$(urlencode "$slug")
else
  urlencoded_slug=$(urlencode "$slug_o")
fi

query="branch=$branch\
       &commit=$commit\
       &build=$([ "$build_o" = "" ] && echo "$build" || echo "$build_o")\
       &build_url=$build_url\
       &name=$(urlencode "$name")\
       &tag=$([ "$tag_o" = "" ] && echo "$tag" || echo "$tag_o")\
       &slug=$urlencoded_slug\
       &service=$service\
       &flags=$flags\
       &pr=$([ "$pr_o" = "" ] && echo "${pr##\#}" || echo "${pr_o##\#}")\
       &job=$job\
       &cmd_args=$(IFS=,; echo "${codecov_flags[*]}")"

if [ -n "$project" ] && [ -n "$server_uri" ];
then
  query=$(echo "$query&project=$project&server_uri=$server_uri" | tr -d ' ')
fi

if [ "$parent" != "" ];
then
  query=$(echo "parent=$parent&$query" | tr -d ' ')
fi

if [ "$ft_search" = "1" ];
then
  # detect bower components location
  bower_components="bower_components"
  bower_rc=$(cd "$git_root" && cat .bowerrc 2>/dev/null || echo "")
  if [ "$bower_rc" != "" ];
  then
    bower_components=$(echo "$bower_rc" | tr -d '\n' | grep '"directory"' | cut -d'"' -f4 | sed -e 's/\/$//')
    if [ "$bower_components" = "" ];
    then
      bower_components="bower_components"
    fi
  fi

  # Swift Coverage
  if [ "$ft_xcodellvm" = "1" ] && [ -d "$ddp" ];
  then
    say "${e}==>${x} Processing Xcode reports via llvm-cov"
    say "    DerivedData folder: $ddp"
    profdata_files=$(find "$ddp" -name '*.profdata' 2>/dev/null || echo '')
    if [ "$profdata_files" != "" ];
    then
      # xcode via profdata
      if [ "$xp" = "" ];
      then
        # xp=$(xcodebuild -showBuildSettings 2>/dev/null | grep -i "^\s*PRODUCT_NAME" | sed -e 's/.*= \(.*\)/\1/')
        # say " ${e}->${x} Speed up Xcode processing by adding ${e}-J '$xp'${x}"
        say "    ${g}hint${x} Speed up Swift processing by using use ${g}-J 'AppName'${x} (regexp accepted)"
        say "    ${g}hint${x} This will remove Pods/ from your report. Also ${b}https://docs.codecov.io/docs/ignoring-paths${x}"
      fi
      while read -r profdata;
      do
        if [ "$profdata" != "" ];
        then
          swiftcov "$profdata" "$xp"
        fi
      done <<< "$profdata_files"
    else
      say "    ${e}->${x} No Swift coverage found"
    fi

    # Obj-C Gcov Coverage
    if [ "$ft_gcov" = "1" ];
    then
      say "    ${e}->${x} Running $gcov_exe for Obj-C"
      if [ "$ft_gcovout" = "0" ];
      then
        # suppress gcov output
        bash -c "find $ddp -type f -name '*.gcda' $gcov_include $gcov_ignore -exec $gcov_exe -p $gcov_arg {} +" >/dev/null 2>&1 || true
      else
        bash -c "find $ddp -type f -name '*.gcda' $gcov_include $gcov_ignore -exec $gcov_exe -p $gcov_arg {} +" || true
      fi
    fi
  fi

  if [ "$ft_xcodeplist" = "1" ] && [ -d "$ddp" ];
  then
    say "${e}==>${x} Processing Xcode plists"
    plists_files=$(find "$ddp" -name '*.xccoverage' 2>/dev/null || echo '')
    if [ "$plists_files" != "" ];
    then
      while read -r plist;
      do
        if [ "$plist" != "" ];
        then
          say "    ${g}Found${x} plist file at $plist"
          plutil -convert xml1 -o "$(basename "$plist").plist" -- "$plist"
        fi
      done <<< "$plists_files"
    fi
  fi

  # Gcov Coverage
  if [ "$ft_gcov" = "1" ];
  then
    say "${e}==>${x} Running $gcov_exe in $proj_root ${e}(disable via -X gcov)${x}"
    if [ "$ft_gcovout" = "0" ];
    then
      # suppress gcov output
      bash -c "find $proj_root -type f -name '*.gcno' $gcov_include $gcov_ignore -exec $gcov_exe -pb $gcov_arg {} +" >/dev/null 2>&1 || true
    else
      bash -c "find $proj_root -type f -name '*.gcno' $gcov_include $gcov_ignore -exec $gcov_exe -pb $gcov_arg {} +" || true
    fi
  else
    say "${e}==>${x} gcov disabled"
  fi

  # Python Coverage
  if [ "$ft_coveragepy" = "1" ];
  then
    if [ ! -f coverage.xml ];
    then
      if command -v coverage >/dev/null 2>&1;
      then
        say "${e}==>${x} Python coveragepy exists ${e}disable via -X coveragepy${x}"

        dotcoverage=$(find "$git_root" -name '.coverage' -or -name '.coverage.*' | head -1 || echo '')
        if [ "$dotcoverage" != "" ];
        then
          cd "$(dirname "$dotcoverage")"
          if [ ! -f .coverage ];
          then
            say "    ${e}->${x} Running coverage combine"
            coverage combine -a
          fi
          say "    ${e}->${x} Running coverage xml"
          if [ "$(coverage xml -i)" != "No data to report." ];
          then
            files="$files
$PWD/coverage.xml"
          else
            say "    ${r}No data to report.${x}"
          fi
          cd "$proj_root"
        else
          say "    ${r}No .coverage file found.${x}"
        fi
      else
        say "${e}==>${x} Python coveragepy not found"
      fi
    fi
  else
    say "${e}==>${x} Python coveragepy disabled"
  fi

  if [ "$search_in_o" != "" ];
  then
    # location override
    search_in="$search_in_o"
  fi

  say "$e==>$x Searching for coverage reports in:"
  for _path in $search_in
  do
    say "    ${g}+${x} $_path"
  done

  patterns="find $search_in \( \
                        -name vendor \
                        -or -name '$bower_components' \
                        -or -name '.egg-info*' \
                        -or -name 'conftest_*.c.gcov' \
                        -or -name .env \
                        -or -name .envs \
                        -or -name .git \
                        -or -name .hg \
                        -or -name .tox \
                        -or -name .venv \
                        -or -name .venvs \
                        -or -name .virtualenv \
                        -or -name .virtualenvs \
                        -or -name .yarn-cache \
                        -or -name __pycache__ \
                        -or -name env \
                        -or -name envs \
                        -or -name htmlcov \
                        -or -name js/generated/coverage \
                        -or -name node_modules \
                        -or -name venv \
                        -or -name venvs \
                        -or -name virtualenv \
                        -or -name virtualenvs \
                    \) -prune -or \
                    -type f \( -name '*coverage*.*' \
                     -or -name '*.clover' \
                     -or -name '*.codecov.*' \
                     -or -name '*.gcov' \
                     -or -name '*.lcov' \
                     -or -name '*.lst' \
                     -or -name 'clover.xml' \
                     -or -name 'cobertura.xml' \
                     -or -name 'codecov.*' \
                     -or -name 'cover.out' \
                     -or -name 'codecov-result.json' \
                     -or -name 'coverage-final.json' \
                     -or -name 'excoveralls.json' \
                     -or -name 'gcov.info' \
                     -or -name 'jacoco*.xml' \
                     -or -name '*Jacoco*.xml' \
                     -or -name 'lcov.dat' \
                     -or -name 'lcov.info' \
                     -or -name 'luacov.report.out' \
                     -or -name 'naxsi.info' \
                     -or -name 'nosetests.xml' \
                     -or -name 'report.xml' \
                     $include_cov \) \
                    $exclude_cov \
                    -not -name '*.am' \
                    -not -name '*.bash' \
                    -not -name '*.bat' \
                    -not -name '*.bw' \
                    -not -name '*.cfg' \
                    -not -name '*.class' \
                    -not -name '*.cmake' \
                    -not -name '*.cmake' \
                    -not -name '*.conf' \
                    -not -name '*.coverage' \
                    -not -name '*.cp' \
                    -not -name '*.cpp' \
                    -not -name '*.crt' \
                    -not -name '*.css' \
                    -not -name '*.csv' \
                    -not -name '*.csv' \
                    -not -name '*.data' \
                    -not -name '*.db' \
                    -not -name '*.dox' \
                    -not -name '*.ec' \
                    -not -name '*.ec' \
                    -not -name '*.egg' \
                    -not -name '*.el' \
                    -not -name '*.env' \
                    -not -name '*.erb' \
                    -not -name '*.exe' \
                    -not -name '*.ftl' \
                    -not -name '*.gif' \
                    -not -name '*.gradle' \
                    -not -name '*.gz' \
                    -not -name '*.h' \
                    -not -name '*.html' \
                    -not -name '*.in' \
                    -not -name '*.jade' \
                    -not -name '*.jar*' \
                    -not -name '*.jpeg' \
                    -not -name '*.jpg' \
                    -not -name '*.js' \
                    -not -name '*.less' \
                    -not -name '*.log' \
                    -not -name '*.m4' \
                    -not -name '*.mak*' \
                    -not -name '*.md' \
                    -not -name '*.o' \
                    -not -name '*.p12' \
                    -not -name '*.pem' \
                    -not -name '*.png' \
                    -not -name '*.pom*' \
                    -not -name '*.profdata' \
                    -not -name '*.proto' \
                    -not -name '*.ps1' \
                    -not -name '*.pth' \
                    -not -name '*.py' \
                    -not -name '*.pyc' \
                    -not -name '*.pyo' \
                    -not -name '*.rb' \
                    -not -name '*.rsp' \
                    -not -name '*.rst' \
                    -not -name '*.ru' \
                    -not -name '*.sbt' \
                    -not -name '*.scss' \
                    -not -name '*.scss' \
                    -not -name '*.serialized' \
                    -not -name '*.sh' \
                    -not -name '*.snapshot' \
                    -not -name '*.sql' \
                    -not -name '*.svg' \
                    -not -name '*.tar.tz' \
                    -not -name '*.template' \
                    -not -name '*.whl' \
                    -not -name '*.xcconfig' \
                    -not -name '*.xcoverage.*' \
                    -not -name '*/classycle/report.xml' \
                    -not -name '*codecov.yml' \
                    -not -name '*~' \
                    -not -name '.*coveragerc' \
                    -not -name '.coverage*' \
                    -not -name 'coverage-summary.json' \
                    -not -name 'createdFiles.lst' \
                    -not -name 'fullLocaleNames.lst' \
                    -not -name 'include.lst' \
                    -not -name 'inputFiles.lst' \
                    -not -name 'phpunit-code-coverage.xml' \
                    -not -name 'phpunit-coverage.xml' \
                    -not -name 'remapInstanbul.coverage*.json' \
                    -not -name 'scoverage.measurements.*' \
                    -not -name 'test_*_coverage.txt' \
                    -not -name 'testrunner-coverage*' \
                    -print 2>/dev/null"
  files=$(eval "$patterns" || echo '')

elif [ "$include_cov" != "" ];
then
  files=$(eval "find $search_in -type f \( ${include_cov:5} \)$exclude_cov 2>/dev/null" || echo '')
elif [ "$direct_file_upload" != "" ];
then
  files=$direct_file_upload
fi

num_of_files=$(echo "$files" | wc -l | tr -d ' ')
if [ "$num_of_files" != '' ] && [ "$files" != '' ];
then
  say "    ${e}->${x} Found $num_of_files reports"
fi

# no files found
if [ "$files" = "" ];
then
  say "${r}-->${x} No coverage report found."
  say "    Please visit ${b}http://docs.codecov.io/docs/supported-languages${x}"
  exit ${exit_with};
fi

if [ "$ft_network" == "1" ];
then
  say "${e}==>${x} Detecting git/mercurial file structure"
  network=$(cd "$git_root" && git ls-files $git_ls_files_recurse_submodules_o 2>/dev/null || hg locate 2>/dev/null || echo "")
  if [ "$network" = "" ];
  then
    network=$(find "$git_root" \( \
                   -name virtualenv \
                   -name .virtualenv \
                   -name virtualenvs \
                   -name .virtualenvs \
                   -name '*.png' \
                   -name '*.gif' \
                   -name '*.jpg' \
                   -name '*.jpeg' \
                   -name '*.md' \
                   -name .env \
                   -name .envs \
                   -name env \
                   -name envs \
                   -name .venv \
                   -name .venvs \
                   -name venv \
                   -name venvs \
                   -name .git \
                   -name .egg-info \
                   -name shunit2-2.1.6 \
                   -name vendor \
                   -name __pycache__ \
                   -name node_modules \
                   -path "*/$bower_components/*" \
                   -path '*/target/delombok/*' \
                   -path '*/build/lib/*' \
                   -path '*/js/generated/coverage/*' \
                    \) -prune -or \
                    -type f -print 2>/dev/null || echo '')
  fi

  if [ "$network_filter_o" != "" ];
  then
      network=$(echo "$network" | grep -e "$network_filter_o/*")
  fi
  if [ "$prefix_o" != "" ];
  then
      network=$(echo "$network" | awk "{print \"$prefix_o/\"\$0}")
  fi
fi

upload_file=$(mktemp /tmp/codecov.XXXXXX)
adjustments_file=$(mktemp /tmp/codecov.adjustments.XXXXXX)

cleanup() {
    rm -f "$upload_file" "$adjustments_file" "$upload_file.gz"
}

trap cleanup INT ABRT TERM


if [ "$env" != "" ];
then
  inc_env=""
  say "${e}==>${x} Appending build variables"
  for varname in $(echo "$env" | tr ',' ' ')
  do
    if [ "$varname" != "" ];
    then
      say "    ${g}+${x} $varname"
      inc_env="${inc_env}${varname}=$(eval echo "\$${varname}")
"
    fi
  done
  echo "$inc_env<<<<<< ENV" >> "$upload_file"
fi

# Append git file list
# write discovered yaml location
if [ "$direct_file_upload" = "" ];
then
  echo "$yaml" >> "$upload_file"
fi

if [ "$ft_network" == "1" ];
then
  i="woff|eot|otf"  # fonts
  i="$i|gif|png|jpg|jpeg|psd"  # images
  i="$i|ptt|pptx|numbers|pages|md|txt|xlsx|docx|doc|pdf|csv"  # docs
  i="$i|.gitignore"  # supporting docs

  if [ "$ft_html" != "1" ];
  then
    i="$i|html"
  fi

  if [ "$ft_yaml" != "1" ];
  then
    i="$i|yml|yaml"
  fi

  echo "$network" | grep -vwE "($i)$" >> "$upload_file"
fi
echo "<<<<<< network" >> "$upload_file"

if [ "$direct_file_upload" = "" ];
then
  fr=0
  say "${e}==>${x} Reading reports"
  while IFS='' read -r file;
  do
    # read the coverage file
    if [ "$(echo "$file" | tr -d ' ')" != '' ];
    then
      if [ -f "$file" ];
      then
        report_len=$(wc -c < "$file")
        if [ "$report_len" -ne 0 ];
        then
          say "    ${g}+${x} $file ${e}bytes=$(echo "$report_len" | tr -d ' ')${x}"
          # append to to upload
          _filename=$(basename "$file")
          if [ "${_filename##*.}" = 'gcov' ];
          then
            {
              echo "# path=$(echo "$file.reduced" | sed "s|^$git_root/||")";
              # get file name
              head -1 "$file";
            } >> "$upload_file"
            # 1. remove source code
            # 2. remove ending bracket lines
            # 3. remove whitespace
            # 4. remove contextual lines
            # 5. remove function names
            awk -F': *' '{print $1":"$2":"}' "$file" \
              | sed '\/: *} *$/d' \
              | sed 's/^ *//' \
              | sed '/^-/d' \
              | sed 's/^function.*/func/' >> "$upload_file"
          else
            {
              echo "# path=${file//^$git_root/||}";
              cat "$file";
            } >> "$upload_file"
          fi
          echo "<<<<<< EOF" >> "$upload_file"
          fr=1
          if [ "$clean" = "1" ];
          then
            rm "$file"
          fi
        else
          say "    ${r}-${x} Skipping empty file $file"
        fi
      else
        say "    ${r}-${x} file not found at $file"
      fi
    fi
  done <<< "$(echo -e "$files")"

  if [ "$fr" = "0" ];
  then
    say "${r}-->${x} No coverage data found."
    say "    Please visit ${b}http://docs.codecov.io/docs/supported-languages${x}"
    say "    search for your projects language to learn how to collect reports."
    exit ${exit_with};
  fi
else
  cp "$direct_file_upload" "$upload_file"
  if [ "$clean" = "1" ];
  then
    rm "$direct_file_upload"
  fi
fi

if [ "$ft_fix" = "1" ];
then
  say "${e}==>${x} Appending adjustments"
  say "    ${b}https://docs.codecov.io/docs/fixing-reports${x}"

  empty_line='^[[:space:]]*$'
  # //
  syntax_comment='^[[:space:]]*//.*'
  # /* or */
  syntax_comment_block='^[[:space:]]*(\/\*|\*\/)[[:space:]]*$'
  # { or }
  syntax_bracket='^[[:space:]]*[\{\}][[:space:]]*(//.*)?$'
  # [ or ]
  syntax_list='^[[:space:]]*[][][[:space:]]*(//.*)?$'
  # func ... {
  syntax_go_func='^[[:space:]]*[func].*[\{][[:space:]]*$'

  # shellcheck disable=SC2089
  skip_dirs="-not -path '*/$bower_components/*' \
             -not -path '*/node_modules/*'"

  cut_and_join() {
    awk 'BEGIN { FS=":" }
         $3 ~ /\/\*/ || $3 ~ /\*\// { print $0 ; next }
         $1!=key { if (key!="") print out ; key=$1 ; out=$1":"$2 ; next }
         { out=out","$2 }
         END { print out }' 2>/dev/null
  }

  if echo "$network" | grep -m1 '.kt$' 1>/dev/null;
  then
    # skip brackets and comments
    cd "$git_root" && \
      find . -type f \
             -name '*.kt' \
             -exec \
      grep -nIHE -e "$syntax_bracket" \
                 -e "$syntax_comment_block" {} \; \
      | cut_and_join \
      >> "$adjustments_file" \
      || echo ''

    # last line in file
    cd "$git_root" && \
      find . -type f \
             -name '*.kt' -exec \
      wc -l {} \; \
      | while read -r l; do echo "EOF: $l"; done \
      2>/dev/null \
      >> "$adjustments_file" \
      || echo ''
  fi

  if echo "$network" | grep -m1 '.go$' 1>/dev/null;
  then
    # skip empty lines, comments, and brackets
    cd "$git_root" && \
      find . -type f \
             -not -path '*/vendor/*' \
             -not -path '*/caches/*' \
             -name '*.go' \
             -exec \
      grep -nIHE \
           -e "$empty_line" \
           -e "$syntax_comment" \
           -e "$syntax_comment_block" \
           -e "$syntax_bracket" \
           -e "$syntax_go_func" \
           {} \; \
      | cut_and_join \
      >> "$adjustments_file" \
      || echo ''
  fi

  if echo "$network" | grep -m1 '.dart$' 1>/dev/null;
  then
    # skip brackets
    cd "$git_root" && \
      find . -type f \
             -name '*.dart' \
             -exec \
      grep -nIHE \
           -e "$syntax_bracket" \
           {} \; \
      | cut_and_join \
      >> "$adjustments_file" \
      || echo ''
  fi

  if echo "$network" | grep -m1 '.php$' 1>/dev/null;
  then
    # skip empty lines, comments, and brackets
    cd "$git_root" && \
      find . -type f \
             -not -path "*/vendor/*" \
             -name '*.php' \
             -exec \
      grep -nIHE \
           -e "$syntax_list" \
           -e "$syntax_bracket" \
           -e '^[[:space:]]*\);[[:space:]]*(//.*)?$' \
           {} \; \
      | cut_and_join \
      >> "$adjustments_file" \
      || echo ''
  fi

  if echo "$network" | grep -m1 '\(.c\.cpp\|.cxx\|.h\|.hpp\|.m\|.swift\|.vala\)$' 1>/dev/null;
  then
    # skip brackets
    # shellcheck disable=SC2086,SC2090
    cd "$git_root" && \
      find . -type f \
             $skip_dirs \
         \( \
           -name '*.c' \
           -or -name '*.cpp' \
           -or -name '*.cxx' \
           -or -name '*.h' \
           -or -name '*.hpp' \
           -or -name '*.m' \
           -or -name '*.swift' \
           -or -name '*.vala' \
         \) -exec \
      grep -nIHE \
           -e "$empty_line" \
           -e "$syntax_bracket" \
           -e '// LCOV_EXCL' \
           {} \; \
      | cut_and_join \
      >> "$adjustments_file" \
      || echo ''

    # skip brackets
    # shellcheck disable=SC2086,SC2090
    cd "$git_root" && \
      find . -type f \
             $skip_dirs \
         \( \
           -name '*.c' \
           -or -name '*.cpp' \
           -or -name '*.cxx' \
           -or -name '*.h' \
           -or -name '*.hpp' \
           -or -name '*.m' \
           -or -name '*.swift' \
           -or -name '*.vala' \
         \) -exec \
      grep -nIH '// LCOV_EXCL' \
           {} \; \
      >> "$adjustments_file" \
      || echo ''

  fi

  found=$(< "$adjustments_file" tr -d ' ')

  if [ "$found" != "" ];
  then
    say "    ${g}+${x} Found adjustments"
    {
      echo "# path=fixes";
      cat "$adjustments_file";
      echo "<<<<<< EOF";
    } >> "$upload_file"
    rm -rf "$adjustments_file"
  else
    say "    ${e}->${x} No adjustments found"
  fi
fi

if [ "$url_o" != "" ];
then
  url="$url_o"
fi

if [ "$dump" != "0" ];
then
  # trim whitespace from query
  say "    ${e}->${x} Dumping upload file (no upload)"
  echo "$url/upload/v4?$(echo "package=$package-$VERSION&token=$token&$query" | tr -d ' ')"
  cat "$upload_file"
else
  if [ "$save_to" != "" ];
  then
    say "${e}==>${x} Copying upload file to ${save_to}"
    mkdir -p "$(dirname "$save_to")"
    cp "$upload_file" "$save_to"
  fi

  say "${e}==>${x} Gzipping contents"
  gzip -nf9 "$upload_file"
  say "        $(du -h "$upload_file.gz")"

  query=$(echo "${query}" | tr -d ' ')
  say "${e}==>${x} Uploading reports"
  say "    ${e}url:${x} $url"
  say "    ${e}query:${x} $query"

  # Full query without token (to display on terminal output)
  queryNoToken=$(echo "package=$package-$VERSION&token=secret&$query" | tr -d ' ')
  # now add token to query
  query=$(echo "package=$package-$VERSION&token=$token&$query" | tr -d ' ')

  if [ "$ft_s3" = "1" ];
  then
    say "${e}->${x}  Pinging Codecov"
    say "$url/upload/v4?$queryNoToken"
    # shellcheck disable=SC2086,2090
    res=$(curl $curl_s -X POST $cacert \
          --retry 5 --retry-delay 2 --connect-timeout 2 \
          -H 'X-Reduced-Redundancy: false' \
          -H 'X-Content-Type: application/x-gzip' \
          -H 'Content-Length: 0' \
          --write-out "\n%{response_code}\n" \
          $curlargs \
          "$url/upload/v4?$query" || true)
    # a good reply is "https://codecov.io" + "\n" + "https://storage.googleapis.com/codecov/..."
    s3target=$(echo "$res" | sed -n 2p)
    status=$(tail -n1 <<< "$res")

    if [ "$status" = "200" ] && [ "$s3target" != "" ];
    then
      say "${e}->${x}  Uploading to"
      say "${s3target}"

      # shellcheck disable=SC2086
      s3=$(curl -fiX PUT \
          --data-binary @"$upload_file.gz" \
          -H 'Content-Type: application/x-gzip' \
          -H 'Content-Encoding: gzip' \
          $curlawsargs \
          "$s3target" || true)

      if [ "$s3" != "" ];
      then
        say "    ${g}->${x} Reports have been successfully queued for processing at ${b}$(echo "$res" | sed -n 1p)${x}"
        exit 0
      else
        say "    ${r}X>${x} Failed to upload"
      fi
    elif [ "$status" = "400" ];
    then
        # 400 Error
        say "${r}${res}${x}"
        exit ${exit_with}
    else
        say "${r}${res}${x}"
    fi
  fi

  say "${e}==>${x} Uploading to Codecov"

  # shellcheck disable=SC2086,2090
  res=$(curl -X POST $cacert \
        --data-binary @"$upload_file.gz" \
        --retry 5 --retry-delay 2 --connect-timeout 2 \
        -H 'Content-Type: text/plain' \
        -H 'Content-Encoding: gzip' \
        -H 'X-Content-Encoding: gzip' \
        -H 'Accept: text/plain' \
        $curlargs \
        "$url/upload/v2?$query&attempt=$i" || echo 'HTTP 500')
  # {"message": "Coverage reports upload successfully", "uploaded": true, "queued": true, "id": "...", "url": "https://codecov.io/..."\}
  uploaded=$(grep -o '\"uploaded\": [a-z]*' <<< "$res" | head -1 | cut -d' ' -f2)
  if [ "$uploaded" = "true" ]
  then
    say "    Reports have been successfully queued for processing at ${b}$(echo "$res" | head -2 | tail -1)${x}"
    exit 0
  else
    say "    ${g}${res}${x}"
    exit ${exit_with}
  fi

  say "    ${r}X> Failed to upload coverage reports${x}"
fi

exit ${exit_with}
