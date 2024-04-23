## List Elemental images

The script `listimages.sh` is meant to list the Elemental images referenced
in charts to stdout. As arguments one can provide the elemental-operator version and/or
the registry URL to use a suffix for image references. For instance:

```
$ ./listimages.sh -o 1.4.3 -r registry.opensuse.org/isv/rancher/elemental/dev/containers
```

By default uses the parent tag, if any, as the elemental-operator version and `registry.suse.com`
as the default registry URL.

The script is meant to be used to generate text file listing all relevant images for a given
elemental-operator release.

## Check Elemental images exist in registry

The script `checkimages.sh` can be used to check the existence of a provided list of images.
It simply checks image per image they are found in registry. In addition if the image name
ends with the `channel` suffix it assumes this is a channel image and it tries to fetch
and also test the images referenced in the channel itself. For insance:

```
$ ./listimages.sh -o 1.4.3 | ./checkimages.sh
```

The above checks version v1.4.3 is available in registry.suse.com and also checks
the referenced images in channels are also available.

## Create `elemental-operator-images.txt` file for releases

`listimages.sh` and `checkimages.sh` scripts can be used to create and
verify an `elemental-operator-images.txt` file that lists all elemental-operator
related images. This file can be manually included on each elemental-operator release.
