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
Source:         %{name}-%{version}.tar.gz

BuildRequires:  gcc-c++
BuildRequires:  glibc-devel
BuildRequires:  openssl-devel
BuildRequires:  golang-packaging
BuildRequires:  make
BuildRequires:  sed
BuildRequires:  golang(API) >= 1.16

BuildRoot:      %{_tmppath}/%{name}-%{version}-build
%{go_provides}

%package -n elemental-register
Summary: operator-register client

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

%prep
%setup -q -n %{name}-%{version}

%build

%goprep .

mkdir -p bin
if [ "$(uname)" = "Linux" ]; then
    OTHER_LINKFLAGS="-extldflags -static -s"
fi

export GIT_TAG=`echo "%{version}" | cut -d "+" -f 1`
export GIT_COMMIT=`echo "%{version}" | cut -d "." -f 4`
export COMMITDATE=`echo %{version} | cut -d "+" -f 2 | cut -d "." -f 1`

# build helm chart - disabled in favor of elemental-operator-helm package
#REPO=registry.opensuse.org/isv/Rancher/Elemental/images/15.3/rancher/%{name} TAG=latest make chart

# build binaries
CGO_ENABLED=0 make operator
CGO_ENABLED=1 make register
make support

%install
%goinstall

# /usr/sbin
%{__install} -d -m 755 %{buildroot}/%{_sbindir}


# binary
%{__install} -m 755 build/elemental-operator %{buildroot}%{_sbindir}
%{__install} -m 755 build/elemental-register %{buildroot}%{_sbindir}
%{__install} -m 755 build/elemental-support %{buildroot}%{_sbindir}

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


%changelog
