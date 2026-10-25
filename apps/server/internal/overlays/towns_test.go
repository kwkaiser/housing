package overlays

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
)

func TestTowns(t *testing.T) {
	places := `{"type":"FeatureCollection","features":[
		{"type":"Feature","properties":{"STATE":"25","PLACE":"62535","NAME":"Somerville city","AREALAND":10671516},
		 "geometry":{"type":"Polygon","coordinates":[[[-71.123456789,42.37],[-71.07,42.37],[-71.07,42.41],[-71.123456789,42.37]]]}},
		{"type":"Feature","properties":{"STATE":"25","PLACE":"00000","NAME":"Nowhere"},"geometry":null},
		{"type":"Feature","properties":{"STATE":"25","PLACE":"11111","NAME":"Sliver"},
		 "geometry":{"type":"Polygon","coordinates":[[[-71,42],[-71,42.1],[-71,42]]]}}
	]}`
	townships := `{"type":"FeatureCollection","features":[
		{"type":"Feature","properties":{"STATE":"25","COUSUB":"62535","NAME":"Somerville city (township)","AREALAND":10671516},
		 "geometry":{"type":"Polygon","coordinates":[[[-71.12,42.37],[-71.07,42.37],[-71.07,42.41],[-71.12,42.37]]]}},
		{"type":"Feature","properties":{"STATE":"25","COUSUB":"01605","NAME":"Arlington town","AREALAND":13340000},
		 "geometry":{"type":"MultiPolygon","coordinates":[
			[[[-71.2,42.4],[-71.1,42.4],[-71.1,42.45],[-71.2,42.4]],[[-71.15,42.41],[-71.12,42.41],[-71.12,42.42],[-71.15,42.41]]],
			[[[-71.3,42.4],[-71.25,42.4],[-71.25,42.45],[-71.3,42.4]]]
		 ]}}
	]}`

	got, err := Towns([]byte(places), []byte(townships))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "town:2501605" || got[1].ID != "town:2562535" {
		t.Fatalf("towns = %+v", got)
	}
	arl, som := got[0], got[1]
	if arl.Label != "Arlington town" || len(arl.Polygons) != 2 || len(arl.Polygons[0]) != 2 || len(arl.Lines) != 0 {
		t.Errorf("arlington = %+v", arl)
	}
	if som.Label != "Somerville city" || som.Area != 10671516 {
		t.Errorf("somerville = %+v, want the place to win over its township", som)
	}
	if p := som.Polygons[0][0][0]; p != (mvt.LatLng{42.37, -71.12346}) {
		t.Errorf("somerville first point = %v, want rounded lat/lng", p)
	}

	if _, err := Towns([]byte(`{"features":[{"properties":{"STATE":"25","PLACE":"1"},"geometry":{"type":"Polygon","coordinates":"nope"}}]}`)); err == nil {
		t.Error("bad coordinates should fail")
	}
}
