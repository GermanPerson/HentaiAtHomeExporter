# HentaiAtHome Exporter
This tool makes H@H network and client statistics available in Prometheus format.

## Usage
Set the following environment variables:
- `EH_USERNAME`: Your E-Hentai username
- `EH_PASSWORD`: Your E-Hentai password

Run the exporter, either by building it yourself, using a recent CI-built binary or using the Docker image.

The metrics are available on port 2112, at path `/metrics`.
