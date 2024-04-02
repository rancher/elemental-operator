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

%define minorv 1.5

Name:           elemental-operator%{minorv}
Version:        0
Release:        0
Summary:        Kubernetes operator to support OS management
License:        Apache-2.0
Group:          System/Management
URL:            https://github.com/rancher/%{name}
Source:         %{name}-%{version}.tar
Source1:        %{name}.obsinfo

Provides: elemental-operator
Conflicts: elemental-operator < %{minorv}

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

%define systemdir /system
%define oemdir %{systemdir}/oem

%description
The Elemental operator is responsible for managing the OS
versions and maintaining a machine inventory to assist with edge or
baremetal installations.

%package -n elemental-register%{minorv}
Summary: The registration client

Provides: elemental-register
Conflicts: elemental-register < %{minorv}

%description -n elemental-register%{minorv}
The elemental-register command is responsible of the node registration
against an elemental-operator instance running under Rancher.

%package -n elemental-support%{minorv}
Summary: Collect important logs for support

Provides: elemental-support
Conflicts: elemental-support < %{minorv}

%description -n elemental-support%{minorv}
This collects essential configuration files and logs to improve issue
resolution.

%package -n elemental-httpfy%{minorv}
Summary: Simple http server

Provides: elemental-httpfy
Conflicts: elemental-httpfy < %{minorv}

%description -n elemental-httpfy%{minorv}
httpfy starts a simple http server, serving files from the current dir.

%package -n elemental-seedimage-hooks%{minorv}
Summary: Hooks used in SeedImage builder

Provides: elemental-seedimage-hooks
Conflicts: elemental-seedimage-hooks < %{minorv}

%description -n elemental-seedimage-hooks%{minorv}
Hooks used in SeedImage builder to copy firmware when building disk-images.

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

# hooks
mkdir -p %{buildroot}%{oemdir}
%{__install} -m 755 hooks/*.yaml %{buildroot}%{oemdir}/

%files
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-operator

%files -n elemental-register%{minorv}
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-register

%files -n elemental-support%{minorv}
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-support


%files -n elemental-httpfy%{minorv}
%defattr(-,root,root,-)
%license LICENSE
%{_sbindir}/elemental-httpfy

%files -n elemental-seedimage-hooks%{minorv}
%defattr(-,root,root,-)
%license LICENSE
%dir %{systemdir}
%dir %{oemdir}
%{oemdir}/*

%changelog
