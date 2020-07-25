# shapeyourcity-map-dump

shapeyourcity-map-dump dumps CSV marker and response data from a data file created by [shapeyourcity-map-sync](../shapeyourcity-map-sync).

The following columns are always dumped:

* id: the marker ID
* url: a direct URL to the marker
* created_at: UTC time when the marker was created
* address: civic address for the marker
* user: login of user who created marker
* category: marker category
* lat, lng: latitude and logitude of marker

Extra columns for responses can also be dumped. See Usage below.

## Usage

```
shapeyourcity-map-dump
```

This will dump the default columns above form `data.db`.

The data file path can be changed with the `-database-file` option.

For extra response columns, more arguments can be added. For example:

```
shapeyourcity-map-dump your_commment '^Your Comment' what_should_happen '^What should'
```

Would dump the default columns, followed by a `your_comment` column for the marker question beginning with
`Your Comment` and a `what_should_happen` column for the marker question beginning with `What should`.
