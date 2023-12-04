#
# spec file for package elemental-operator
#
# Copyright (c) 2022 SUSE LLC
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


Name:           elemental-operator
Version:        0
Release:        0
Summary:        Kubernetes operator to support OS management
License:        Apache-2.0
Group:          System/Management
URL:            https://github.com/rancher/%{name}
Source:         %{name}-%{version}.tar
Source1:        %{name}.obsinfo

BuildRequires:  gcc-c++
BuildRequires:  glibc-devel
BuildRequires:  openssl-devel
BuildRequires:  make
BuildRequires:  grep

%if 0%{?suse_version}
BuildRequires:  golang(API) >= 1.20
BuildRequires:  golang-packaging
%{go_provides}
%else
%global goipath    google.golang.org/api
%global forgeurl   https://github.com/rancher/elemental-operator
%global commit     25abcdc57b9409d4c5b2009cf0a2f9aa6ff647ad
%gometa
%if (0%{?centos_version} == 800) || (0%{?rhel_version} == 800)
BuildRequires:  go1.20
%else
BuildRequires:  compiler(go-compiler) >= 1.20
%endif
%endif

BuildRoot:      %{_tmppath}/%{name}-%{version}-build

%package -n elemental-register
Summary: The registration client

%description
The Elemental operator is responsible for managing the OS
versions and maintaining a machine inventory to assist with edge or
baremetal installations.

%description -n elemental-register
The elemental-register command is responsible of the node registration
against an elemental-operator instance running under Rancher.

%package -n elemental-support
Summary: Collect important logs for support

%description -n elemental-support
This collects essential configuration files and logs to improve issue
resolution.

%package -n elemental-httpfy
Summary: Simple http server

%description -n elemental-httpfy
httpfy starts a simple http server, serving files from the current dir.

%prep
%setup -q -n %{name}-%{version}
cp %{S:1} .

%build
%if 0%{?suse_version}
%goprep .
%endif

mkdir -p bin
if [ "$(uname)" = "Linux" ]; then
    OTHER_LINKFLAGS="-extldflags -static -s"
fi

export GIT_TAG=`echo "%{version}" | cut -d "+" -f 1`
GIT_COMMIT=$(cat %{name}.obsinfo | grep commit: | cut -d" " -f 2)
export GIT_COMMIT=${GIT_COMMIT:0:8}
MTIME=$(cat %{name}.obsinfo | grep mtime: | cut -d" " -f 2)
export COMMITDATE=$(date -d @${MTIME} +%Y%m%d)

# build binaries
CGO_ENABLED=0 make operator
CGO_ENABLED=1 make register
make support
make httpfy

%install
%if 0%{?suse_version}
%goinstall
%endif

# /usr/sbin
%{__install} -d -m 755 %{buildroot}/%{_sbindir}


# binary
%{__install} -m 755 build/elemental-operator %{buildroot}%{_sbindir}
%{__install} -m 755 build/elemental-register %{buildroot}%{_sbindir}
%{__install} -m 755 build/elemental-support %{buildroot}%{_sbindir}
%{__install} -m 755 build/elemental-httpfy %{buildroot}%{_sbindir}

%files
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/%{name}

%files -n elemental-register
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-register

%files -n elemental-support
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-support


%files -n elemental-httpfy
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-httpfy

%changelog
