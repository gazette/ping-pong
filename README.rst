Example: Ping-Pong
===================

Ping-pong is a simple Gazette consumer application that is a useful starting
point for scaffolding out new Gazette consumer application projects.

Repository Layout
-------------------

- ``ping_pong.go`` implements the complete application, and ``ping_pong_test.go``
  provides (very basic) end-to-end test coverage.
- ``ping_pong.proto`` implements a ``Volley`` protobuf message type. Use of
  protobuf in this example is overkill -- it could just as easily be a Go struct --
  but is included to demonstrate building applications with Protobuf support.
  ``ping_pong.pb.go`` is checked-in, associated generated code.
- ``Makefile`` provides an opinionated, complete build system for the application.
  It leverages Gazette's build infrastructure to include support for RocksDB,
  a hermetic Docker-based build environment, targets for ``go install``
  and ``go test``, and targets for packaging production-ready release images.
- ``kustomize`` provides Kubernetes manifests for deploying the application,
  including manifests for end-to-end testing.

Makefile
---------

Many Gazette consumers -- including this one -- can be trivially built and tested
with ``go install`` & ``go test``. For regular static Go binaries, a program
compiled for Linux can be packaged using any suitable Docker base image such
as alpine. If you would rather not use this Makefile, feel free to junk it!

Things get more interesting for consumer applications which depend on RocksDB
(i.e, try adding ``import _ "go.gazette.dev/core/consumer/store-rocksdb"``
to this application).

RocksDB introduces CGO and compile time dependencies on a number of libraries,
as well as shared library runtime dependencies which must ultimately be packaged
with the release image. Gazette's use of RocksDB also requires compile-time flags
(notably enabling run-time type information) which are not enabled by the rocksdb
Debian package.

The ``Makefile`` of this repository determines the local directory holding the
active ``go.gazette.dev/core`` module from ``go.mod``, and includes Makefiles of
that repository designed to be re-used by external applications. **You must
first run ``go mod download`` before you'll be able to use any of these
targets**. Just about any other go command will work instead, as long as it
causes the modules to be downloaded. It provides several useful targets:

:go-install:
   Fetch and build RocksDB if required (it's compiled into a ``.build`` subdirectory
   of the repo checkout).

   Invoke ``protoc`` to regenerate any protobuf messages which have been updated.

   *Then*, invoke ``go install`` with appropriate CGO flags to find and use RocksDB.

:go-test-fast:
    Invoke ``go test`` to run all tests one time, with appropriate CGO flags for RocksDB.

:go-test-ci:
    Invoke ``go test`` in continuous integration mode, running tests 15 times with
    race detection enabled.

:as-ci:
    Invoke Make recursively, inside a hermetic Docker-based build environment
    suitable for continuous integration.

    You must also pass a ``target`` flag which is the actual target to invoke
    within the hermetic builder, like ``make as-ci target=go-install``.

    ``as-ci`` bind-mounts the local repo checkout into the Docker container.
    If you're running Docker For Mac or Windows, you may need to explicitly enable
    sharing of the relevant host volumes.

:ci-release-ping-pong:
    Build a release-ready Docker image of the ping-pong application, ``ping-pong:latest``.
    The image includes RocksDB and dependencies atop a minimal Ubuntu base image.

    This target should generally be invoked using `as-ci`:
    ``make as-ci target=ci-release-ping-pong``.

:push-to-registry:
    Push a built ``ping-pong:latest`` image to a configurable registry,
    defaulting to ``localhost:32000``.

Kustomize
-----------

Manifests in this repo re-use base manifests from ``go.gazette.dev/core``,
which are expected to be available as ``./kustomize/core``.

As a first step, soft-link the current gazette module path to that directory:

.. code-block:: console

    $ ln -s $(go list -f '{{ .Dir }}' -m go.gazette.dev/core)/kustomize kustomize/core

You'll need to update this soft-link whenever you change the associated ``go.mod`` module version.
(There's probably a better way to manage this, but I don't know it off hand).

Manifests of this repo are:

:kustomize/bases/ping-pong:
    Base manifest which kustomizes the ``consumer`` manifest of the Gazette repo to
    the ping-pong application.

:kustomize/test/deploy-ping-pong:
    Test manifest which deploys ``ping-pong`` and all dependencies to a ``ping-pong``
    namespace of the target Kubernetes cluster. It also includes jobs to create
    associated JournalSpecs and ShardSpecs.

    Invoke as ``kubectl apply -k ./kustomize/test/deploy-ping-pong``.

