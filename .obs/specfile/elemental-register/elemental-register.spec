#
# spec file for package elemental-register
#
# Copyright (c) 2025 SUSE LLC
#
# All modifications and additions to the file contributed by third parties
# remain the property of their copyright owners, unless otherwise agreed
# upon. The license for this file, and modifications and additions to the
# file, is the same license as for the pristine package itself (unless the
# license for the pristine package is not an Open Source License, in which
# case the license is the MIT License). An "Open Source License" is a
# license that conforms to the Open Source Definition (Version 1.9)
# published by the Open Source Initiative.

# Please submit bugfixes or comments via https://bugs.opensuse.org/
#

%define commit _replaceme_
%define c_date _replaceme_

Name:           elemental-register
Version:        0
Release:        0
Summary:        The Elemental Operator registration client
License:        Apache-2.0
Group:          System/Management
URL:            https://github.com/rancher/elemental-operator
Source:         %{name}.tar.xz

# go-tpm-tools aren't _that_ portable :-(
ExclusiveArch:  x86_64 aarch64

BuildRequires:  gcc-c++
BuildRequires:  glibc-devel
BuildRequires:  openssl-devel
BuildRequires:  make
BuildRequires:  grep

%if 0%{?suse_version}
BuildRequires:  golang(API) >= 1.22
BuildRequires:  golang-packaging
%{go_provides}
%else
%global goipath    google.golang.org/api
%global forgeurl   https://github.com/rancher/elemental-operator
%global commit     25abcdc57b9409d4c5b2009cf0a2f9aa6ff647ad
%gometa
%if (0%{?centos_version} == 800) || (0%{?rhel_version} == 800)
BuildRequires:  go1.22
%else
BuildRequires:  compiler(go-compiler) >= 1.22
%endif
%endif

BuildRoot:      %{_tmppath}/%{name}-%{version}-build

%description
The elemental-register command is responsible of the node registration
against an elemental-operator instance running under Rancher.

%package -n elemental-support
Summary: Collect important logs for support

%description -n elemental-support
This collects essential configuration files and logs to improve issue
resolution.

%prep
%setup -q -n %{name}

%build
%if 0%{?suse_version}
%goprep .
%endif

mkdir -p bin
if [ "$(uname)" = "Linux" ]; then
    OTHER_LINKFLAGS="-extldflags -static -s"
fi

if [ "%{commit}" = "_replaceme_" ]; then
  echo "No commit hash provided"
  exit 1
fi

if [ "%{c_date}" = "_replaceme_" ]; then
  echo "No commit date provided"
  exit 1
fi

export GIT_TAG=$(echo "%{version}" | cut -d "+" -f 1)
GIT_COMMIT=$(echo "%{commit}")
export GIT_COMMIT=${GIT_COMMIT:0:8}
export COMMITDATE="%{c_date}"

# build binaries
CGO_ENABLED=1 make register
make support


%install
%if 0%{?suse_version}
%goinstall
%endif

# /usr/sbin
%{__install} -d -m 755 %{buildroot}/%{_sbindir}

# binary
%{__install} -m 755 build/elemental-register %{buildroot}%{_sbindir}
%{__install} -m 755 build/elemental-support %{buildroot}%{_sbindir}

%files
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-register

%files -n elemental-support
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-support


%changelog
