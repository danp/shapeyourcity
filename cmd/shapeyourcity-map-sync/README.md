# shapeyourcity-map-sync

shapeyourcity-map-sync syncs marker and response data from a Shape Your City map, such as the [Halifax Mobility Response Streets & Spaces](https://www.shapeyourcityhalifax.ca/mobilityresponse/maps/halifax-mobility-response-streets) map.

Data is stored in a [SQLite](https://sqlite.org/) file which makes it easy to do ad-hoc queries on the data using the [CLI](https://sqlite.org/cli.html).

## Usage

For example:

```
shapeyourcity-map-sync -base-url https://www.shapeyourcityhalifax.ca/mobilityresponse/maps/halifax-mobility-response-streets
```

This will sync markers and responses from the Halifax Mobility Response Streets & Spaces map, creating or updating `data.db` in the current directory.

The data file path can be changed with the `-database-file` option.
